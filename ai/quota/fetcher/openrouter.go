package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// OpenRouterFetcher retrieves OpenRouter quota data.
// Uses: GET https://openrouter.ai/api/v1/key (the key's spending cap and usage)
// and GET https://openrouter.ai/api/v1/credits (the account's credits).
type OpenRouterFetcher struct{}

func NewOpenRouterFetcher() *OpenRouterFetcher {
	return &OpenRouterFetcher{}
}

func (f *OpenRouterFetcher) Name() string                     { return "openrouter" }
func (f *OpenRouterFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeOpenRouter }
func (f *OpenRouterFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *OpenRouterFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// openrouterKeyResponse from GET /api/v1/key
type openrouterKeyResponse struct {
	Data struct {
		Label            string   `json:"label"`
		IsFreeTier       bool     `json:"is_free_tier"`
		Limit            *float64 `json:"limit"`           // nullable
		LimitRemaining   *float64 `json:"limit_remaining"` // nullable
		Usage            float64  `json:"usage"`
		UsageDaily       float64  `json:"usage_daily"`
		UsageWeekly      float64  `json:"usage_weekly"`
		UsageMonthly     float64  `json:"usage_monthly"`
		ByokUsage        float64  `json:"byok_usage"`
		ByokUsageDaily   float64  `json:"byok_usage_daily"`
		ByokUsageWeekly  float64  `json:"byok_usage_weekly"`
		ByokUsageMonthly float64  `json:"byok_usage_monthly"`
		ExpiresAt        *string  `json:"expires_at"`
		CreatorUserID    string   `json:"creator_user_id"`
	} `json:"data"`
}

// openrouterCreditsResponse from GET /api/v1/credits: all-time totals for the
// account behind the key. The docs reserve the endpoint for management keys,
// but inference keys have been answered too, so it is read best-effort.
type openrouterCreditsResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"` // purchased
		TotalUsage   float64 `json:"total_usage"`   // spent
	} `json:"data"`
}

func (f *OpenRouterFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// OpenRouter providers are configured with the inference base
	// ("https://openrouter.ai/api/v1", or "…/api" for the Anthropic style);
	// the key endpoint is addressed from the root, so the prefix comes off
	// before the full path goes on.
	root := apiRoot(provider.APIBase, "https://openrouter.ai", "/api/v1", "/api")

	keyBody, err := openrouterGet(ctx, client, root+"/api/v1/key", token)
	if err != nil {
		return nil, err
	}
	var keyResp openrouterKeyResponse
	if err := json.Unmarshal(keyBody, &keyResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// No answer (403 for a key without management rights, or any failure)
	// leaves the balance out rather than failing the key's figures.
	var credits *openrouterCreditsResponse
	creditsBody, err := openrouterGet(ctx, client, root+"/api/v1/credits", token)
	if err == nil {
		var decoded openrouterCreditsResponse
		if json.Unmarshal(creditsBody, &decoded) == nil {
			credits = &decoded
		} else {
			creditsBody = nil
		}
	}

	rawResponse, err := json.Marshal(struct {
		Key     json.RawMessage `json:"key"`
		Credits json.RawMessage `json:"credits"`
	}{keyBody, creditsBody})
	if err != nil {
		return nil, fmt.Errorf("encode raw response: %w", err)
	}

	data := keyResp.Data
	now := time.Now()

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenRouter,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(rawResponse),
	}

	// Account info
	tier := "paid"
	if data.IsFreeTier {
		tier = "free"
	}
	usage.Account = &quota.UsageAccount{
		ID:   data.CreatorUserID,
		Tier: tier,
	}

	// The account's credits are the money requests are paid from. The balance
	// can dip below zero, which OpenRouter answers with 402 even for free
	// models; the window clamps at zero and the description keeps the sign.
	if credits != nil {
		purchased, spent := credits.Data.TotalCredits, credits.Data.TotalUsage
		balance := purchased - spent
		available := max(balance, 0)
		usage.AddWindow("balance", &quota.UsageWindow{
			Type:         quota.WindowTypeBalance,
			Used:         spent,
			Limit:        purchased,
			Available:    &available,
			Unit:         quota.UsageUnitCurrency,
			CurrencyCode: "USD",
			Label:        "Balance",
			Description: fmt.Sprintf("Balance: %s (purchased $%.2f, used $%.2f)",
				signedUSD(balance), purchased, spent),
		})
	}

	// A key limit is a spending cap on this key, not money: the key stops at
	// the cap even with credits left, and running out of credits stops it
	// under the cap. Raising the cap is the only way back, so it is a
	// resource. limit_remaining is OpenRouter's own figure (it leaves BYOK
	// usage out unless include_byok_in_limit); usage is the fallback.
	// limit_reset (daily/weekly/monthly caps) is not modelled yet.
	if data.Limit != nil && *data.Limit > 0 {
		limit := *data.Limit
		available := limit - data.Usage
		if data.LimitRemaining != nil {
			available = *data.LimitRemaining
		}
		available = min(max(available, 0), limit)
		usage.AddWindow("key_limit", &quota.UsageWindow{
			Type:         quota.WindowTypeCustom,
			Kind:         quota.WindowKindResource,
			Used:         limit - available,
			Limit:        limit,
			Available:    &available,
			Unit:         quota.UsageUnitCurrency,
			CurrencyCode: "USD",
			Label:        "Key Limit",
			Description:  fmt.Sprintf("Key limit: $%.2f left of $%.2f", available, limit),
		})
	}

	// Monthly spend is never capped — the key limit caps lifetime usage, not
	// the month — so this window has no cap in either branch and says so.
	// A bare Limit of 0 would be read as a budget with nothing spent.
	monthly := &quota.UsageWindow{
		Type:          quota.WindowTypeMonthly,
		Used:          data.UsageMonthly,
		Unlimited:     true,
		Unit:          quota.UsageUnitCurrency,
		WindowMinutes: 30 * 24 * 60,
		Label:         "Monthly",
		Description: fmt.Sprintf("Daily: $%.4f | Weekly: $%.4f | Monthly: $%.4f | Total: $%.4f",
			data.UsageDaily, data.UsageWeekly, data.UsageMonthly, data.Usage),
	}
	if data.Limit == nil || *data.Limit <= 0 {
		monthly.Label = "Monthly Usage"
		monthly.Description = fmt.Sprintf("This month: $%.4f (no limit set)", data.UsageMonthly)
	}
	usage.AddWindow("monthly", monthly)

	// Cost
	usage.Cost = &quota.UsageCost{
		Used:         data.Usage,
		CurrencyCode: "USD",
		Label:        "Total Usage",
	}
	if data.Limit != nil {
		usage.Cost.Limit = *data.Limit
	}

	return usage, nil
}

// openrouterGet sends an authenticated GET and returns the body of a 200.
func openrouterGet(ctx context.Context, client *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

// signedUSD formats an amount that may be negative as "-$0.01", not "$-0.01".
func signedUSD(v float64) string {
	if v < 0 {
		return fmt.Sprintf("-$%.2f", -v)
	}
	return fmt.Sprintf("$%.2f", v)
}
