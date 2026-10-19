package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestOpenRouterFetcher_Fetch(t *testing.T) {
	const response = `{"data":{"label":"sk-or-v1-test","is_free_tier":false,"limit":100,"usage":35.5,"usage_daily":1.2,"usage_weekly":12.3,"usage_monthly":30,"byok_usage":0,"byok_usage_daily":0,"creator_user_id":"user_test123"}}`
	const creditsResponse = `{"data":{"total_credits":120,"total_usage":35.5}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/key":
			_, _ = w.Write([]byte(response))
		case "/api/v1/credits":
			_, _ = w.Write([]byte(creditsResponse))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{}
	provider := &ai.Provider{
		UUID:    "test-uuid",
		Name:    "OpenRouter",
		Token:   "test-key",
		APIBase: server.URL,
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if usage.ProviderUUID != "test-uuid" {
		t.Errorf("ProviderUUID = %q, want test-uuid", usage.ProviderUUID)
	}
	if usage.ProviderType != quota.ProviderTypeOpenRouter {
		t.Errorf("ProviderType = %q, want openrouter", usage.ProviderType)
	}
	var raw struct {
		Key     json.RawMessage `json:"key"`
		Credits json.RawMessage `json:"credits"`
	}
	if err := json.Unmarshal(usage.RawResponse, &raw); err != nil {
		t.Fatalf("RawResponse is not JSON: %v", err)
	}
	if string(raw.Key) != response || string(raw.Credits) != creditsResponse {
		t.Errorf("RawResponse = %s, want both upstream bodies verbatim", usage.RawResponse)
	}

	// Balance window: the account's credits
	balance := findWindow(t, usage, "balance")
	if balance.Type != quota.WindowTypeBalance {
		t.Errorf("Balance.Type = %q, want balance", balance.Type)
	}
	if balance.Available == nil || *balance.Available != 84.5 {
		t.Errorf("Balance.Available = %v, want 84.5", balance.Available)
	}
	if balance.Limit != 120 || balance.Used != 35.5 {
		t.Errorf("Balance = %v/%v, want 35.5 used of 120 purchased", balance.Used, balance.Limit)
	}

	// Key limit window: a cap on the key, not a balance
	keyLimit := findWindow(t, usage, "key_limit")
	if keyLimit.Type == quota.WindowTypeBalance {
		t.Errorf("KeyLimit.Type = %q, want a non-balance type", keyLimit.Type)
	}
	if keyLimit.Used != 35.50 {
		t.Errorf("KeyLimit.Used = %f, want 35.50", keyLimit.Used)
	}
	if keyLimit.Limit != 100.0 {
		t.Errorf("KeyLimit.Limit = %f, want 100.0", keyLimit.Limit)
	}
	if keyLimit.Unit != quota.UsageUnitCurrency {
		t.Errorf("KeyLimit.Unit = %q, want currency", keyLimit.Unit)
	}

	// Monthly window
	monthly := findWindow(t, usage, "monthly")
	if monthly.Type != quota.WindowTypeMonthly {
		t.Errorf("Monthly.Type = %q, want monthly", monthly.Type)
	}
	if monthly.Used != 30.00 {
		t.Errorf("Monthly.Used = %f, want 30.00", monthly.Used)
	}

	// Cost
	if usage.Cost == nil {
		t.Fatal("Cost is nil")
	}
	if usage.Cost.Used != 35.50 {
		t.Errorf("Cost.Used = %f, want 35.50", usage.Cost.Used)
	}
	if usage.Cost.Limit != 100.0 {
		t.Errorf("Cost.Limit = %f, want 100.0", usage.Cost.Limit)
	}

	// Account
	if usage.Account == nil {
		t.Fatal("Account is nil")
	}
	if usage.Account.ID != "user_test123" {
		t.Errorf("Account.ID = %q, want user_test123", usage.Account.ID)
	}
	if usage.Account.Tier != "paid" {
		t.Errorf("Account.Tier = %q, want paid", usage.Account.Tier)
	}
}

func TestOpenRouterFetcher_FreeTier(t *testing.T) {
	usage := openrouterUsage(t, `{"data":{"label":"sk-or-v1-free","is_free_tier":true,"limit":null,`+
		`"usage":0,"usage_daily":0,"usage_weekly":0,"usage_monthly":0,"creator_user_id":"user_free"}}`)

	// No limit and no credits answer → first window shows monthly usage
	if len(usage.Windows) == 0 {
		t.Fatal("expected quota windows")
	}
	if usage.Windows[0].Type != quota.WindowTypeMonthly {
		t.Errorf("First window Type = %q, want monthly (no limit)", usage.Windows[0].Type)
	}
	if usage.Account.Tier != "free" {
		t.Errorf("Account.Tier = %q, want free", usage.Account.Tier)
	}
	if usage.Cost.Limit != 0 {
		t.Errorf("Cost.Limit = %f, want 0 (no limit)", usage.Cost.Limit)
	}
}

func TestOpenRouterFetcher_StatusError(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{}
	provider := &ai.Provider{
		UUID:    "test-uuid",
		Name:    "OpenRouter",
		Token:   "bad-key",
		APIBase: server.URL,
	}

	_, err := fetcher.Fetch(context.Background(), provider)
	if err == nil {
		t.Fatal("expected error for 401 status")
	}
}

func TestOpenRouterFetcher_Validate(t *testing.T) {
	fetcher := &OpenRouterFetcher{}

	// nil provider
	if err := fetcher.Validate(nil); err == nil {
		t.Fatal("expected error for nil provider")
	}

	// no token
	if err := fetcher.Validate(&ai.Provider{}); err == nil {
		t.Fatal("expected error for empty token")
	}

	// valid
	if err := fetcher.Validate(&ai.Provider{Token: "sk-xxx"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// openrouterUsage fetches with /api/v1/credits answering 403, which the docs
// give for a key without management rights.
func openrouterUsage(t *testing.T, response string) *quota.ProviderUsage {
	t.Helper()
	return openrouterUsageWithCredits(t, response, "")
}

// openrouterUsageWithCredits serves response from /api/v1/key and credits
// from /api/v1/credits; an empty credits answers 403.
func openrouterUsageWithCredits(t *testing.T, response, credits string) *quota.ProviderUsage {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/key":
			_, _ = w.Write([]byte(response))
		case r.URL.Path == "/api/v1/credits" && credits != "":
			_, _ = w.Write([]byte(credits))
		case r.URL.Path == "/api/v1/credits":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Only management keys can perform this operation"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	usage, err := (&OpenRouterFetcher{}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "OpenRouter", Token: "k", APIBase: server.URL})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	return usage
}

func TestOpenRouterKeyLimitIsAResource(t *testing.T) {
	usage := openrouterUsage(t,
		`{"data":{"limit":20,"usage":8.1,"usage_monthly":8.1,"creator_user_id":"u1"}}`)

	checkInvariants(t, usage)

	keyLimit := findWindow(t, usage, "key_limit")
	if keyLimit.Kind != quota.WindowKindResource {
		t.Errorf("key_limit Kind = %q, want resource", keyLimit.Kind)
	}
	if keyLimit.Available == nil || *keyLimit.Available != 11.9 {
		t.Errorf("key_limit Available = %v, want 11.9", keyLimit.Available)
	}
	if keyLimit.CurrencyCode != "USD" {
		t.Errorf("key_limit CurrencyCode = %q, want USD", keyLimit.CurrencyCode)
	}
	if pct, ok := usage.Pct(); !ok || pct < 40.4 || pct > 40.6 {
		t.Fatalf("Pct() = %v, %v; want ~40.5, true", pct, ok)
	}
	// Raising the cap is the only way back, so waiting brings nothing back.
	if usage.RecoversAt() != nil {
		t.Error("RecoversAt() should be nil when the binding entry is a spending cap")
	}
}

// The reported case: a $50 cap on the key showed as "Balance: $50.00 / $50.00"
// on an account that never bought credits and had spent slightly below zero.
func TestOpenRouterBalanceIsTheAccountsCredits(t *testing.T) {
	usage := openrouterUsageWithCredits(t,
		`{"data":{"limit":50,"limit_remaining":50,"usage":0,"is_free_tier":true,"creator_user_id":"u1"}}`,
		`{"data":{"total_credits":0,"total_usage":0.007911}}`)

	checkInvariants(t, usage)

	balance := findWindow(t, usage, "balance")
	if balance.Type != quota.WindowTypeBalance || balance.Kind != quota.WindowKindResource {
		t.Errorf("balance Type/Kind = %q/%q, want balance/resource", balance.Type, balance.Kind)
	}
	if balance.Available == nil || *balance.Available != 0 {
		t.Errorf("balance Available = %v, want 0 (clamped)", balance.Available)
	}
	if balance.Countable() {
		t.Error("nothing was purchased, so there is no share of credits to report")
	}
	if !strings.Contains(balance.Description, "-$0.01") {
		t.Errorf("balance Description = %q, want the signed balance -$0.01", balance.Description)
	}

	keyLimit := findWindow(t, usage, "key_limit")
	if keyLimit.Type == quota.WindowTypeBalance {
		t.Error("the key limit is a cap on the key, not a balance")
	}
	if keyLimit.Available == nil || *keyLimit.Available != 50 {
		t.Errorf("key_limit Available = %v, want 50", keyLimit.Available)
	}
	if strings.Contains(keyLimit.Description, "Balance") {
		t.Errorf("key_limit Description = %q, must not call the cap a balance", keyLimit.Description)
	}
}

// limit - usage misses what OpenRouter itself counts against the cap (resets,
// BYOK accounting); limit_remaining is its own figure, so it wins.
func TestOpenRouterKeyLimitPrefersLimitRemaining(t *testing.T) {
	usage := openrouterUsage(t,
		`{"data":{"limit":20,"limit_remaining":15,"usage":8.1,"creator_user_id":"u1"}}`)

	keyLimit := findWindow(t, usage, "key_limit")
	if keyLimit.Available == nil || *keyLimit.Available != 15 {
		t.Errorf("key_limit Available = %v, want 15", keyLimit.Available)
	}
	if keyLimit.Used != 5 {
		t.Errorf("key_limit Used = %v, want 5", keyLimit.Used)
	}
}

func TestOpenRouterNoKeyLimitMeansUncapped(t *testing.T) {
	// A bare Limit of 0 was ambiguous: no cap, or a cap we could not read.
	usage := openrouterUsage(t,
		`{"data":{"limit":null,"usage":8.1,"usage_monthly":8.1,"creator_user_id":"u1"}}`)

	checkInvariants(t, usage)

	monthly := findWindow(t, usage, "monthly")
	if !monthly.Unlimited {
		t.Error("with no key limit there is no cap; the window must say so")
	}
	if monthly.Countable() {
		t.Error("an uncapped window has no usage proportion to contribute")
	}
	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown for an uncapped key", pct, ok)
	}
}

func TestOpenRouterMonthlySpendNeverPosesAsACap(t *testing.T) {
	// The key limit caps lifetime usage, not the month, so the monthly window
	// has no cap even when a key limit is set. Left unflagged it read as
	// "$8.10 of $0.00" and, being an allowance, sorted ahead of the real one.
	usage := openrouterUsage(t,
		`{"data":{"limit":20,"usage":8.1,"usage_monthly":8.1,"creator_user_id":"u1"}}`)

	checkInvariants(t, usage)

	monthly := findWindow(t, usage, "monthly")
	if monthly.Countable() {
		t.Error("monthly spend has no cap; it must not contribute a usage figure")
	}
	if got := usage.Tightest().Key; got != "key_limit" {
		t.Errorf("Tightest() = %q; want key_limit, the window that actually has a cap", got)
	}
	if usage.Windows[0].Key != "key_limit" {
		t.Errorf("first window = %q; want key_limit ahead of the uncapped one", usage.Windows[0].Key)
	}
}

// A provider's APIBase is its inference endpoint, which for OpenRouter already
// carries the API prefix. Appending the key path to it unstripped produced
// /api/v1/api/v1/key, which OpenRouter answers with a 404 from its web app.
func TestOpenRouterFetcherDoesNotDoubleAPIPrefix(t *testing.T) {
	const response = `{"data":{"label":"k","is_free_tier":true,"usage":1}}`

	for _, suffix := range []string{"", "/", "/api/v1", "/api/v1/", "/api"} {
		t.Run("base"+suffix, func(t *testing.T) {
			var gotPaths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()

			_, err := (&OpenRouterFetcher{}).Fetch(context.Background(), &ai.Provider{
				UUID: "u", Name: "OpenRouter", Token: "k", APIBase: server.URL + suffix,
			})
			if err != nil {
				t.Fatalf("Fetch() error: %v", err)
			}
			if strings.Join(gotPaths, " ") != "/api/v1/key /api/v1/credits" {
				t.Errorf("paths = %q, want /api/v1/key then /api/v1/credits", gotPaths)
			}
		})
	}
}
