package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Upgrade-safety tests for the rules-in-SQLite storage after the config.json
// mirror was removed: whatever shape a user's install is in, their rules must
// survive the first start of this version and every restart after it.

func capPtr(i int) *int { return &i }

// richLegacyRules covers every fat column and the encodings most likely to
// lose information in a JSON → column → JSON round trip.
func richLegacyRules() []typ.Rule {
	svc := func(p, m string, tier int) *loadbalance.Service {
		return &loadbalance.Service{Provider: p, Model: m, Weight: 2, Active: true, TimeWindow: 300, Tier: tier, ModelCapacity: capPtr(100)}
	}
	return []typ.Rule{
		{UUID: "u-openai", Scenario: typ.ScenarioOpenAI, RequestModel: "gpt-x", ResponseModel: "gpt-x-r", Description: "plain", Active: true,
			Services: []*loadbalance.Service{svc("prov-a", "m1", 0), svc("prov-b", "m2", 0)},
			LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()},
			Flags:    typ.RuleFlags{CursorCompat: true, SkipUsage: true, CustomUserAgent: "ua/1", UseMaxTokens: true}},
		{UUID: "u-tier", Scenario: typ.ScenarioClaudeCode, RequestModel: "claude-sonnet-x", Active: true, Description: "tiered",
			Services: []*loadbalance.Service{svc("prov-a", "m1", 0), svc("prov-b", "m2", 1)},
			LBTactic: typ.Tactic{Type: loadbalance.TacticTier, Params: typ.DefaultTierParams()}},
		{UUID: "u-smart", Scenario: typ.ScenarioAnthropic, RequestModel: "smart-model", Active: false, SmartEnabled: true,
			Services: []*loadbalance.Service{svc("prov-a", "m1", 0)},
			LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()},
			SmartRouting: []smartrouting.SmartRouting{{Description: "long ctx",
				Ops:      []smartrouting.SmartOp{{UUID: "op1", Position: smartrouting.PositionModel, Operation: "contains", Value: "opus"}},
				Services: []*loadbalance.Service{svc("prov-b", "m2", 0)}}}},
		{UUID: "u-wild", Scenario: typ.ScenarioOpenAI, RequestModel: "*", Active: true, Description: "wildcard 中文 ✓ \"quoted\" <tag>",
			Services: []*loadbalance.Service{svc("prov-b", "m2", 0)},
			LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()}},
	}
}

// seedProviders puts the providers richLegacyRules references into the shared
// database before the first start, the way an upgraded install already has
// them (providers migrated to SQLite long before rules).
func seedProviders(t *testing.T, dir string) {
	t.Helper()
	sm, err := db.NewStoreManager(dir)
	if err != nil {
		t.Fatalf("NewStoreManager: %v", err)
	}
	for _, id := range []string{"prov-a", "prov-b"} {
		if err := sm.Provider().Save(&typ.Provider{UUID: id, Name: id, APIBase: "https://example.invalid/v1", Enabled: true}); err != nil {
			t.Fatalf("seed provider: %v", err)
		}
	}
	if err := sm.Close(); err != nil {
		t.Fatalf("close store manager: %v", err)
	}
}

// userRulesJSON returns the canonical JSON of the rules whose UUID starts with
// "u-" (built-in rules are excluded: migrations legitimately touch them).
func userRulesJSON(t *testing.T, rules []typ.Rule) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range rules {
		if len(r.UUID) > 2 && r.UUID[:2] == "u-" {
			b, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("marshal %s: %v", r.UUID, err)
			}
			out[r.UUID] = string(b)
		}
	}
	return out
}

func assertSameUserRules(t *testing.T, want, got map[string]string, ctx string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: user rule count %d, want %d", ctx, len(got), len(want))
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: rule %s changed\n  want %s\n  got  %s", ctx, id, w, got[id])
		}
	}
}

func userRuleOrder(rules []typ.Rule) []string {
	var ids []string
	for _, r := range rules {
		if len(r.UUID) > 2 && r.UUID[:2] == "u-" {
			ids = append(ids, r.UUID)
		}
	}
	return ids
}

func startCfg(t *testing.T, dir string) *Config {
	t.Helper()
	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir: %v", err)
	}
	return cfg
}

// A pre-database install: every fat field must survive import and every
// later restart bit-for-bit, in the original order, and config.json must end
// up with rules=null and no default_request_id.
func TestUpgradeFromLegacyFile_FidelityAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	legacy := richLegacyRules()
	writeLegacyConfig(t, dir, legacy)
	raw := readConfigJSON(t, dir)
	raw["default_request_id"] = 2
	data, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg := startCfg(t, dir)
	first := userRulesJSON(t, cfg.Rules)
	order := userRuleOrder(cfg.Rules)
	if len(first) != len(legacy) {
		t.Fatalf("imported %d user rules, want %d", len(first), len(legacy))
	}
	for i, r := range legacy {
		if order[i] != r.UUID {
			t.Fatalf("order changed at %d: %v", i, order)
		}
	}
	// Spot-check the data that actually drives routing.
	if r := findRule(cfg.Rules, "u-tier"); r == nil || r.LBTactic.Type != loadbalance.TacticTier || len(r.Services) != 2 || r.Services[1].Tier != 1 || r.Services[0].ModelCapacity == nil || *r.Services[0].ModelCapacity != 100 {
		t.Errorf("u-tier did not survive import: %+v", r)
	}
	if r := findRule(cfg.Rules, "u-smart"); r == nil || !r.SmartEnabled || len(r.SmartRouting) != 1 || len(r.SmartRouting[0].Ops) != 1 {
		t.Errorf("u-smart did not survive import: %+v", r)
	}
	if r := findRule(cfg.Rules, "u-openai"); r == nil || !r.Flags.CursorCompat || r.Flags.CustomUserAgent != "ua/1" {
		t.Errorf("u-openai flags lost: %+v", r)
	}
	cfg.CloseStores()

	disk := readConfigJSON(t, dir)
	if v, ok := disk["rules"]; ok && v != nil {
		t.Errorf("config.json rules = %v, want null", v)
	}
	if _, ok := disk["default_request_id"]; ok {
		t.Error("stale default_request_id survived")
	}

	for i := 0; i < 3; i++ {
		c := startCfg(t, dir)
		assertSameUserRules(t, first, userRulesJSON(t, c.Rules), fmt.Sprintf("restart %d", i+1))
		if got := userRuleOrder(c.Rules); fmt.Sprint(got) != fmt.Sprint(order) {
			t.Errorf("restart %d reordered rules: %v", i+1, got)
		}
		c.CloseStores()
	}
}

// An install that ran the dual-write version: DB and file mirror agree. The
// first start of this version must leave the DB untouched and null the file.
func TestUpgradeFromMirrorEra_DBUntouchedFileNulled(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	writeLegacyConfig(t, dir, richLegacyRules())
	c := startCfg(t, dir)
	want := userRulesJSON(t, c.Rules)
	c.CloseStores()

	// Re-create the mirror the previous version would have left behind.
	mirror := append([]typ.Rule(nil), c.Rules...)
	raw := readConfigJSON(t, dir)
	raw["rules"] = mirror
	raw["default_request_id"] = 3
	data, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	c2 := startCfg(t, dir)
	assertSameUserRules(t, want, userRulesJSON(t, c2.Rules), "mirror-era upgrade")
	c2.CloseStores()
	disk := readConfigJSON(t, dir)
	if v, ok := disk["rules"]; ok && v != nil {
		t.Errorf("mirror not nulled: %v", v)
	}
	if _, ok := disk["default_request_id"]; ok {
		t.Error("default_request_id not cleaned")
	}
}

// If the database refuses the import write, the file's rules must stay put and
// startup must fail loudly — then succeed with the rules once the DB recovers.
func TestUpgradeImportFailureKeepsFileRules(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	c := startCfg(t, dir) // creates the rules table
	c.CloseStores()

	gdb, err := db.OpenSQLite(constant.GetDBFile(dir), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`CREATE TRIGGER block_rules BEFORE INSERT ON rules BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error; err != nil {
		t.Fatal(err)
	}
	// Clear any rules the previous start inserted (built-ins) so the DB is
	// empty and the legacy path triggers.
	if err := gdb.Exec(`DELETE FROM rules`).Error; err != nil {
		t.Fatal(err)
	}

	writeLegacyConfig(t, dir, richLegacyRules())
	if _, err := NewConfigWithDir(dir); err == nil {
		t.Fatal("expected startup to fail when the rules write is rejected")
	}
	disk := readConfigJSON(t, dir)
	arr, ok := disk["rules"].([]interface{})
	if !ok || len(arr) != len(richLegacyRules()) {
		t.Fatalf("file lost its legacy rules after failed import: %v", disk["rules"])
	}

	if err := gdb.Exec(`DROP TRIGGER block_rules`).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	c2 := startCfg(t, dir)
	defer c2.CloseStores()
	if got := len(userRulesJSON(t, c2.Rules)); got != len(richLegacyRules()) {
		t.Errorf("retry imported %d user rules, want %d", got, len(richLegacyRules()))
	}
}

// DB write succeeds but the file write fails: the change must still be in the
// database (not lost), and the next start must be healthy.
func TestSaveFileWriteFailureLeavesDatabaseAuthoritative(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	writeLegacyConfig(t, dir, richLegacyRules())
	cfg := startCfg(t, dir)

	good := cfg.ConfigFile
	cfg.ConfigFile = dir // a directory: WriteFile fails after the store sync
	r := richLegacyRules()[0]
	r.UUID, r.RequestModel = "u-added", "added-model"
	if err := cfg.AddRule(r); err == nil {
		t.Fatal("expected AddRule to report the file write failure")
	}
	cfg.ConfigFile = good
	cfg.CloseStores()

	c2 := startCfg(t, dir)
	defer c2.CloseStores()
	if findRule(c2.Rules, "u-added") == nil {
		t.Error("rule written to the database was lost after a failed file write")
	}
	if got := len(userRulesJSON(t, c2.Rules)); got != len(richLegacyRules())+1 {
		t.Errorf("unexpected user rule count %d", got)
	}
}

// config.json deleted/reset while the database is intact: rules come back.
func TestRulesSurviveMissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	writeLegacyConfig(t, dir, richLegacyRules())
	c := startCfg(t, dir)
	want := userRulesJSON(t, c.Rules)
	c.CloseStores()

	if err := os.Remove(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	c2 := startCfg(t, dir)
	defer c2.CloseStores()
	assertSameUserRules(t, want, userRulesJSON(t, c2.Rules), "config.json removed")
}

// A hot reload (watcher → load) must never fork the working set from the DB,
// whether the file carries null, a stale array, or a hand edit.
func TestHotReloadDoesNotTouchRules(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	writeLegacyConfig(t, dir, richLegacyRules())
	cfg := startCfg(t, dir)
	defer cfg.CloseStores()
	want := userRulesJSON(t, cfg.Rules)

	for name, rules := range map[string]interface{}{
		"null":         nil,
		"empty":        []typ.Rule{},
		"hand-edited":  []typ.Rule{legacyTestRule("u-hand", "hand-model")},
		"stale-subset": richLegacyRules()[:1],
	} {
		raw := readConfigJSON(t, dir)
		raw["rules"] = rules
		raw["default_request_id"] = 7
		data, _ := json.MarshalIndent(raw, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := cfg.load(); err != nil {
			t.Fatalf("%s: reload failed: %v", name, err)
		}
		assertSameUserRules(t, want, userRulesJSON(t, cfg.Rules), "hot reload "+name)
		if cfg.LegacyRules != nil {
			t.Errorf("%s: LegacyRules carried state past hydration", name)
		}
		// And a subsequent Save must not resurrect anything from the file.
		if err := cfg.Save(); err != nil {
			t.Fatalf("%s: Save after reload: %v", name, err)
		}
		if v := readConfigJSON(t, dir)["rules"]; v != nil {
			t.Errorf("%s: Save wrote rules back to file: %v", name, v)
		}
	}
}

// Concurrent rule mutations racing with unrelated Saves: the in-memory set and
// the database must agree afterwards and survive a restart. Run with -race.
func TestConcurrentMutationsPersistConsistently(t *testing.T) {
	dir := t.TempDir()
	seedProviders(t, dir)
	writeLegacyConfig(t, dir, richLegacyRules())
	cfg := startCfg(t, dir)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := richLegacyRules()[0]
			r.UUID, r.RequestModel = fmt.Sprintf("u-c%d", i), fmt.Sprintf("conc-%d", i)
			if err := cfg.AddRule(r); err != nil {
				t.Errorf("AddRule %d: %v", i, err)
			}
			if err := cfg.SetVerbose(i%2 == 0); err != nil {
				t.Errorf("SetVerbose: %v", err)
			}
		}(i)
	}
	wg.Wait()
	want := userRulesJSON(t, cfg.Rules)
	if len(want) != len(richLegacyRules())+8 {
		t.Fatalf("in-memory user rules = %d", len(want))
	}
	cfg.CloseStores()

	c2 := startCfg(t, dir)
	defer c2.CloseStores()
	assertSameUserRules(t, want, userRulesJSON(t, c2.Rules), "after concurrent mutations + restart")
}
