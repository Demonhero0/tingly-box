package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wbBase = "http://localhost:12580/tingly/workbuddy"

func wbReadJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func TestApplyWorkBuddyConfig_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codebuddy", "models.json")
	res := applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"claude-sonnet-5"})
	require.True(t, res.Success, res.Message)
	assert.True(t, res.Created)

	cfg := wbReadJSON(t, path)
	models := cfg["models"].([]interface{})
	require.Len(t, models, 1)
	m := models[0].(map[string]interface{})
	assert.Equal(t, "claude-sonnet-5", m["id"])
	assert.Equal(t, wbBase+"/v1/chat/completions", m["url"])
	assert.Equal(t, "tok", m["apiKey"])
	assert.Equal(t, true, m["supportsToolCall"])
	assert.Equal(t, false, m["supportsImages"])

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestApplyWorkBuddyConfig_PreservesUserContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "custom": {"keep": true},
  "models": [
    {"id": "mine", "url": "https://api.example.com/v1/chat/completions", "apiKey": "k"},
    {"id": "old-tingly", "url": "http://localhost:12580/tingly/workbuddy/v1/chat/completions", "apiKey": "old"}
  ]
}`), 0o640))

	res := applyWorkBuddyConfigAt(path, wbBase, "new", []string{"fresh"})
	require.True(t, res.Success, res.Message)
	assert.True(t, res.Updated)
	assert.NotEmpty(t, res.BackupPath)

	cfg := wbReadJSON(t, path)
	assert.Equal(t, map[string]interface{}{"keep": true}, cfg["custom"])
	var ids []string
	for _, e := range cfg["models"].([]interface{}) {
		ids = append(ids, e.(map[string]interface{})["id"].(string))
	}
	assert.Equal(t, []string{"mine", "fresh"}, ids, "user entry kept, stale managed entry replaced")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "existing permissions preserved")
}

func TestApplyWorkBuddyConfig_SkipsIDOwnedByUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"models":[{"id":"gpt-x","url":"https://api.example.com/v1/chat/completions"}]}`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"gpt-x", "other"})
	require.True(t, res.Success, res.Message)
	assert.Equal(t, []string{"gpt-x"}, res.Skipped)
	assert.Contains(t, res.Message, "gpt-x")

	cfg := wbReadJSON(t, path)
	models := cfg["models"].([]interface{})
	require.Len(t, models, 2)
	assert.Equal(t, "https://api.example.com/v1/chat/completions", models[0].(map[string]interface{})["url"], "user entry untouched")
}

func TestApplyWorkBuddyConfig_KeepsAvailableModelsWhitelistConsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "models":[{"id":"stale","url":"http://h/tingly/workbuddy/v1/chat/completions"}],
  "availableModels":["mine","stale"]
}`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"fresh"})
	require.True(t, res.Success, res.Message)
	cfg := wbReadJSON(t, path)
	assert.Equal(t, []interface{}{"mine", "fresh"}, cfg["availableModels"])
}

func TestApplyWorkBuddyConfig_LeavesInvalidFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	orig := `[{"id":"array-shaped"}]`
	require.NoError(t, os.WriteFile(path, []byte(orig), 0o600))

	res := applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"m"})
	assert.False(t, res.Success)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, orig, string(data))
}

func TestWorkBuddyStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	assert.Equal(t, WorkBuddyAppliedState{}, readWorkBuddyAppliedStateAt(path), "missing file reads as not applied")
	state, _ := CompareWorkBuddyConfig(WorkBuddyAppliedState{}, []string{"a"})
	assert.Equal(t, ClientConfigNotApplied, state)

	require.True(t, applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"a", "b"}).Success)
	applied := readWorkBuddyAppliedStateAt(path)
	assert.True(t, applied.Managed)

	state, diffs := CompareWorkBuddyConfig(applied, []string{"b", "a"})
	assert.Equal(t, ClientConfigApplied, state)
	assert.Empty(t, diffs)

	state, diffs = CompareWorkBuddyConfig(applied, []string{"a", "c"})
	assert.Equal(t, ClientConfigOutdated, state)
	require.Len(t, diffs, 1)
	assert.Equal(t, "models", diffs[0].Key)
}

func TestApplyWorkBuddyConfig_RefusesWhenEveryIDIsTaken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	orig := `{"models":[{"id":"gpt-x","url":"https://api.example.com/v1/chat/completions"},{"id":"old","url":"http://h/tingly/workbuddy/v1/chat/completions"}]}`
	require.NoError(t, os.WriteFile(path, []byte(orig), 0o600))

	res := applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"gpt-x"})
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "gpt-x")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, orig, string(data), "nothing written, stale managed entry kept")
	assert.Empty(t, res.BackupPath)
}

func TestApplyWorkBuddyConfig_AcceptsUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte("\xef\xbb\xbf"+`{"models":[{"id":"mine","url":"https://api.example.com/v1/chat/completions"}]}`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbBase, "tok", []string{"m"})
	require.True(t, res.Success, res.Message)
	cfg := wbReadJSON(t, path)
	require.Len(t, cfg["models"].([]interface{}), 2)
	assert.True(t, readWorkBuddyAppliedStateAt(path).Managed)
}
