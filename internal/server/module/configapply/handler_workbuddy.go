package configapply

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	aiagent "github.com/tingly-dev/tingly-box/ai/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// collectWorkBuddyRuleModels returns the request_models of every active rule in
// the WorkBuddy scenario, deduplicated and in declaration order.
func collectWorkBuddyRuleModels(cfg *config.Config) []string {
	return collectScenarioRuleModels(cfg, typ.ScenarioWorkBuddy)
}

const noWorkBuddyRulesMessage = "Add a model rule for WorkBuddy first — Auto Config writes one WorkBuddy model per rule."

// ApplyWorkBuddyConfigFromState writes the active WorkBuddy-scenario rules into
// ~/.codebuddy/models.json. Like the other agents it does NOT touch routing
// rules, and unlike OpenCode it does not invent a default model: with no rules
// there is nothing to write, which is reported instead of creating a phantom
// entry.
func (h *Handler) ApplyWorkBuddyConfigFromState(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, ApplyWorkBuddyConfigResponse{
			WorkBuddyApplyResult: config.WorkBuddyApplyResult{ApplyResult: config.ApplyResult{Message: "Global config not available"}},
		})
		return
	}
	models := collectWorkBuddyRuleModels(cfg)
	if len(models) == 0 {
		c.JSON(http.StatusOK, ApplyWorkBuddyConfigResponse{
			WorkBuddyApplyResult: config.WorkBuddyApplyResult{ApplyResult: config.ApplyResult{Message: noWorkBuddyRulesMessage}},
		})
		return
	}

	baseURL := middleware.BaseURLFromRequest(c, h.resolvedPort()) + "/tingly/workbuddy"
	result, err := config.ApplyWorkBuddyConfig(baseURL, cfg.GetModelToken(), models)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApplyWorkBuddyConfigResponse{
			WorkBuddyApplyResult: config.WorkBuddyApplyResult{ApplyResult: config.ApplyResult{Message: "Internal error: " + err.Error()}},
		})
		return
	}
	c.JSON(http.StatusOK, ApplyWorkBuddyConfigResponse{WorkBuddyApplyResult: *result})
}

// GetWorkBuddyConfigPreview returns the models.json entries Auto Config would
// write now, with the API key masked.
func (h *Handler) GetWorkBuddyConfigPreview(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, WorkBuddyConfigPreviewResponse{Message: "Global config not available"})
		return
	}
	models := collectWorkBuddyRuleModels(cfg)
	baseURL := middleware.BaseURLFromRequest(c, h.resolvedPort()) + "/tingly/workbuddy"
	entries := config.BuildWorkBuddyModelEntries(baseURL, maskSecret(cfg.GetModelToken()), models)
	out, err := json.MarshalIndent(map[string]interface{}{"models": entries}, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, WorkBuddyConfigPreviewResponse{Message: "Failed to generate config JSON: " + err.Error()})
		return
	}
	resp := WorkBuddyConfigPreviewResponse{Success: true, ConfigJSON: string(out), Path: config.WorkBuddyModelsRelPath, Models: models}
	if len(models) == 0 {
		resp.Message = noWorkBuddyRulesMessage
	}
	c.JSON(http.StatusOK, resp)
}

// GetWorkBuddyConfigStatus compares ~/.codebuddy/models.json with the models the
// WorkBuddy rules produce now.
func (h *Handler) GetWorkBuddyConfigStatus(c *gin.Context) {
	applied, err := config.ReadWorkBuddyAppliedState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ClientConfigStatusResponse{Error: err.Error()})
		return
	}
	state, diffs := config.CompareWorkBuddyConfig(applied, collectWorkBuddyRuleModels(h.config))
	c.JSON(http.StatusOK, ClientConfigStatusResponse{
		Success:     true,
		State:       state,
		Path:        config.WorkBuddyModelsRelPath,
		Differences: toDifferences(diffs),
	})
}

// RestoreWorkBuddyConfig rolls ~/.codebuddy/models.json back to its most recent
// backup (the pre-restore state is itself backed up first).
func (h *Handler) RestoreWorkBuddyConfig(c *gin.Context) {
	const agentType = "workbuddy"
	path, err := config.WorkBuddyModelsFilePath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, RestoreConfigResponse{AgentType: agentType, Message: "Restore failed: " + err.Error()})
		return
	}
	r, err := aiagent.RestoreLatestBackup(path)
	if err != nil {
		msg := err.Error()
		if r != nil && r.Message != "" {
			msg = r.Message
		}
		c.JSON(http.StatusOK, RestoreConfigResponse{
			AgentType: agentType,
			Failures:  []string{config.WorkBuddyModelsRelPath + ": " + msg},
			Message:   "Restore failed for " + agentType + ": " + msg,
		})
		return
	}
	resp := RestoreConfigResponse{
		Success:       true,
		AgentType:     agentType,
		RestoredFiles: []string{config.WorkBuddyModelsRelPath + " <- " + r.RestoredFrom},
		Message:       "Restored " + config.WorkBuddyModelsRelPath,
	}
	if r.PreRestoreBackup != "" {
		resp.PreRestoreBackups = []string{r.PreRestoreBackup}
	}
	c.JSON(http.StatusOK, resp)
}
