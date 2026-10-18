package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WorkBuddy config application: merge tingly-managed model entries into
// ~/.codebuddy/models.json while preserving everything the user put there.
//
// Why ~/.codebuddy and not ~/.workbuddy: ~/.codebuddy/models.json (user level;
// <workspace>/.codebuddy/models.json is project level and wins on the same id)
// is the file Tencent documents — "WorkBuddy Enterprise 模型配置" on Tencent
// Cloud and workbuddy.ai/docs/cli/models — with hot reload. WorkBuddy's own
// desktop doc says models "previously configured via ~/.codebuddy/models.json"
// still work. ~/.workbuddy/models.json appears only in third-party guides.
// Whether the desktop app reads this exact file is not confirmed on a real
// install; see .design/workbuddy-agent.md.

// WorkBuddyModelsRelPath is the display path of the file this writes.
const WorkBuddyModelsRelPath = "~/.codebuddy/models.json"

// workBuddyEndpointMarker tags the entries tingly-box owns: their `url` points at
// this gateway's WorkBuddy scenario. The schema has no free-form metadata field,
// so ownership is derived from the URL instead of an extra key WorkBuddy might
// reject.
const workBuddyEndpointMarker = "/tingly/workbuddy"

// workBuddyChatPath is appended to the scenario base URL; WorkBuddy wants the
// complete chat-completions endpoint.
const workBuddyChatPath = "/v1/chat/completions"

// workBuddyVendor is the group the entries show under in WorkBuddy's model picker.
const workBuddyVendor = "Tingly Box"

func workBuddyModelsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("home directory resolved to empty path")
	}
	return filepath.Join(home, ".codebuddy", "models.json"), nil
}

// BuildWorkBuddyModelEntries renders one models.json entry per model id.
// baseURL is the scenario base (".../tingly/workbuddy"). Capability flags are
// conservative — tool calling on, images and reasoning off — since WorkBuddy
// reports call errors for flags the routed model does not support.
func BuildWorkBuddyModelEntries(baseURL, apiKey string, models []string) []map[string]interface{} {
	url := strings.TrimRight(baseURL, "/") + workBuddyChatPath
	entries := make([]map[string]interface{}, 0, len(models))
	for _, id := range models {
		entries = append(entries, map[string]interface{}{
			"id":                id,
			"name":              id,
			"vendor":            workBuddyVendor,
			"url":               url,
			"apiKey":            apiKey,
			"supportsToolCall":  true,
			"supportsImages":    false,
			"supportsReasoning": false,
		})
	}
	return entries
}

func isWorkBuddyManaged(entry interface{}) bool {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	url, _ := m["url"].(string)
	return strings.Contains(url, workBuddyEndpointMarker+"/")
}

func entryID(entry interface{}) string {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return ""
	}
	id, _ := m["id"].(string)
	return id
}

// WorkBuddyApplyResult extends ApplyResult with the model ids that could not be
// written because the user already has an unrelated entry with the same id.
type WorkBuddyApplyResult struct {
	ApplyResult
	Skipped []string `json:"skipped,omitempty"`
}

// mergeWorkBuddyModels replaces tingly-managed entries in existing with
// desired, never touching other entries. A desired id that collides with a
// user-owned entry is skipped (returned) rather than overwriting it. When an
// availableModels whitelist exists it is kept consistent: stale managed ids are
// dropped and the written ids added, otherwise our models would be filtered out.
func mergeWorkBuddyModels(existing map[string]interface{}, desired []map[string]interface{}) (skipped []string) {
	var kept []interface{}
	userIDs := map[string]struct{}{}
	oldManaged := map[string]struct{}{}
	if models, ok := existing["models"].([]interface{}); ok {
		for _, e := range models {
			if isWorkBuddyManaged(e) {
				oldManaged[entryID(e)] = struct{}{}
				continue
			}
			kept = append(kept, e)
			if id := entryID(e); id != "" {
				userIDs[id] = struct{}{}
			}
		}
	}

	written := map[string]struct{}{}
	for _, d := range desired {
		id, _ := d["id"].(string)
		if _, clash := userIDs[id]; clash {
			skipped = append(skipped, id)
			continue
		}
		kept = append(kept, d)
		written[id] = struct{}{}
	}
	if kept == nil {
		kept = []interface{}{}
	}
	existing["models"] = kept

	if avail, ok := existing["availableModels"].([]interface{}); ok && len(avail) > 0 {
		var next []interface{}
		have := map[string]struct{}{}
		for _, a := range avail {
			if s, ok := a.(string); ok {
				if _, stale := oldManaged[s]; stale {
					if _, still := written[s]; !still {
						continue
					}
				}
				have[s] = struct{}{}
			}
			next = append(next, a)
		}
		ids := make([]string, 0, len(written))
		for id := range written {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			if _, ok := have[id]; !ok {
				next = append(next, id)
			}
		}
		existing["availableModels"] = next
	}
	return skipped
}

// ApplyWorkBuddyConfig merges the tingly-managed entries into
// ~/.codebuddy/models.json. It refuses (touching nothing) when the existing
// file is not a JSON object, backs up before overwriting, and writes
// atomically with 0600 permissions because the file holds API keys.
func ApplyWorkBuddyConfig(baseURL, apiKey string, models []string) (*WorkBuddyApplyResult, error) {
	targetPath, err := workBuddyModelsPath()
	if err != nil {
		return nil, err
	}
	return applyWorkBuddyConfigAt(targetPath, baseURL, apiKey, models), nil
}

func applyWorkBuddyConfigAt(targetPath, baseURL, apiKey string, models []string) *WorkBuddyApplyResult {
	result := &WorkBuddyApplyResult{}
	fail := func(format string, a ...interface{}) *WorkBuddyApplyResult {
		result.Success = false
		result.Message = fmt.Sprintf(format, a...)
		return result
	}

	existing := map[string]interface{}{}
	mode := os.FileMode(0o600)
	info, statErr := os.Stat(targetPath)
	if statErr == nil {
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return fail("Failed to read existing file: %v", err)
		}
		data = trimUTF8BOM(data)
		if len(strings.TrimSpace(string(data))) > 0 {
			if err := json.Unmarshal(data, &existing); err != nil {
				return fail("%s is not a JSON object (%v); left untouched", WorkBuddyModelsRelPath, err)
			}
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return fail("Failed to stat %s: %v", targetPath, statErr)
	}

	result.Skipped = mergeWorkBuddyModels(existing, BuildWorkBuddyModelEntries(baseURL, apiKey, models))
	// Every id is taken by one of the user's own models: writing would only
	// drop our stale entries and add nothing, so refuse before touching the file.
	if len(models) > 0 && len(result.Skipped) == len(models) {
		return fail("Nothing written: %s already has models with ids %s; rename the rule's request model or that model's id",
			WorkBuddyModelsRelPath, strings.Join(result.Skipped, ", "))
	}

	if statErr == nil {
		backupPath, err := backupFile(targetPath)
		if err != nil {
			return fail("Failed to create backup: %v", err)
		}
		result.BackupPath = backupPath
		result.Updated = true
	} else {
		result.Created = true
	}

	output, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fail("Failed to marshal JSON: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fail("Failed to create directory: %v", err)
	}
	if err := writeFileAtomic(targetPath, append(output, '\n'), mode); err != nil {
		return fail("Failed to write file: %v", err)
	}

	result.Success = true
	switch {
	case result.Created:
		result.Message = fmt.Sprintf("Created %s", targetPath)
	case result.BackupPath != "":
		result.Message = fmt.Sprintf("Updated %s (backup: %s)", targetPath, result.BackupPath)
	default:
		result.Message = fmt.Sprintf("Updated %s", targetPath)
	}
	if len(result.Skipped) > 0 {
		result.Message += fmt.Sprintf("; skipped %s (id already used by another model)", strings.Join(result.Skipped, ", "))
	}
	return result
}

// trimUTF8BOM drops a leading UTF-8 byte-order mark. Hand-edited models.json
// files often carry one (Windows editors), and encoding/json rejects it.
func trimUTF8BOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
}

// writeFileAtomic writes via a temp file in the same directory and renames, so
// WorkBuddy's hot reload never sees a half-written file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".models-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// ---- status ----

// WorkBuddyAppliedState is what the status check needs from ~/.codebuddy/models.json.
type WorkBuddyAppliedState struct {
	Managed bool // has at least one entry pointing at this gateway's WorkBuddy scenario
	URL     string
	Models  []string
}

// CompareWorkBuddyConfig compares the applied managed entries with the models
// the current rules would produce.
func CompareWorkBuddyConfig(applied WorkBuddyAppliedState, models []string) (string, []ClientConfigDiff) {
	if !applied.Managed {
		return ClientConfigNotApplied, nil
	}
	var diffs []ClientConfigDiff
	if joinModels(applied.Models) != joinModels(models) {
		diffs = append(diffs, ClientConfigDiff{Key: "models", Applied: joinModels(applied.Models), Expected: joinModels(models)})
	}
	return stateFor(diffs), diffs
}

// ReadWorkBuddyAppliedState reads the tingly-managed entries from
// ~/.codebuddy/models.json. A missing or unparsable file reads as not applied.
func ReadWorkBuddyAppliedState() (WorkBuddyAppliedState, error) {
	path, err := workBuddyModelsPath()
	if err != nil {
		return WorkBuddyAppliedState{}, err
	}
	return readWorkBuddyAppliedStateAt(path), nil
}

func readWorkBuddyAppliedStateAt(path string) WorkBuddyAppliedState {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkBuddyAppliedState{}
	}
	cfg := map[string]interface{}{}
	if json.Unmarshal(trimUTF8BOM(data), &cfg) != nil {
		return WorkBuddyAppliedState{}
	}
	state := WorkBuddyAppliedState{Models: []string{}}
	models, _ := cfg["models"].([]interface{})
	for _, e := range models {
		if !isWorkBuddyManaged(e) {
			continue
		}
		state.Managed = true
		m := e.(map[string]interface{})
		if state.URL == "" {
			state.URL, _ = m["url"].(string)
		}
		if id := entryID(e); id != "" {
			state.Models = append(state.Models, id)
		}
	}
	return state
}

// WorkBuddyModelsFilePath returns the absolute path of the file Auto Config
// writes, for callers that restore it from backup.
func WorkBuddyModelsFilePath() (string, error) { return workBuddyModelsPath() }
