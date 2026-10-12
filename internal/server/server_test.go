package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/project"
)

// fakeAnthropic is a minimal Anthropic Messages endpoint that streams a
// fixed reply and records the requests it got.
type fakeAnthropic struct {
	mu       sync.Mutex
	requests []map[string]any
	// reply, when set, decides the response: text, or a tool call when
	// tool is non-empty.
	reply func(body map[string]any) (text, tool string, input any)
}

func (f *fakeAnthropic) handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/models":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"fake-model","display_name":"Fake Model","type":"model","created_at":"2025-01-01T00:00:00Z"}],"has_more":false,"first_id":"fake-model","last_id":"fake-model"}`)
		return
	case "/v1/messages":
	default:
		http.NotFound(w, r)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.requests = append(f.requests, body)
	f.mu.Unlock()

	reply, tool := "Hello from fake", ""
	var toolInput any
	if f.reply != nil {
		reply, tool, toolInput = f.reply(body)
	} else if strings.Contains(fmt.Sprint(body["messages"]), "Write a short title") {
		reply = "Fake Title"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ev := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
	ev("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","model":"fake-model","content":[],"stop_reason":null,"usage":{"input_tokens":42,"output_tokens":0}}}`)
	if tool != "" {
		in, _ := json.Marshal(toolInput)
		quoted, _ := json.Marshal(string(in))
		ev("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_%d","name":%q,"input":{}}}`, len(f.chatRequests()), tool))
		ev("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%s}}`, quoted))
		ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
		ev("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`)
		ev("message_stop", `{"type":"message_stop"}`)
		return
	}
	ev("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	for _, part := range strings.SplitAfter(reply, " ") {
		b, _ := json.Marshal(part)
		ev("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, b))
	}
	ev("content_block_stop", `{"type":"content_block_stop","index":0}`)
	ev("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`)
	ev("message_stop", `{"type":"message_stop"}`)
}

func (f *fakeAnthropic) chatRequests() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.requests...)
}

type client struct {
	t    *testing.T
	base string
}

func (c client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	if out != nil {
		require.NoError(c.t, json.NewDecoder(resp.Body).Decode(out))
	}
	return resp.StatusCode
}

func (c client) stream(path string, body any) []map[string]any {
	c.t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(c.base+path, "application/json", bytes.NewReader(b))
	require.NoError(c.t, err)
	defer resp.Body.Close()
	require.Equal(c.t, 200, resp.StatusCode)
	require.Equal(c.t, "text/event-stream", resp.Header.Get("Content-Type"))
	var events []map[string]any
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev map[string]any
		require.NoError(c.t, json.Unmarshal([]byte(line[6:]), &ev))
		events = append(events, ev)
	}
	return events
}

func types(events []map[string]any) []string {
	var out []string
	for _, e := range events {
		if e["type"] != "delta" || len(out) == 0 || out[len(out)-1] != "delta" {
			out = append(out, e["type"].(string))
		}
	}
	return out
}

func newServer(t *testing.T, runner project.Runner) (*db.Store, client) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := httptest.NewServer(New(ctx, Deps{Store: store, Layout: project.Layout{Root: t.TempDir()}, Runner: runner}))
	t.Cleanup(srv.Close)
	return store, client{t: t, base: srv.URL + "/api/v1"}
}

func TestChatEndToEnd(t *testing.T) {
	fake := &fakeAnthropic{}
	upstream := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer upstream.Close()
	_, c := newServer(t, nil)

	require.Equal(t, 200, c.do("PUT", "/settings", map[string]any{
		"api_style": "anthropic", "base_url": upstream.URL, "api_key": "sk-test-123456789", "default_model": "",
	}, nil))
	var st map[string]any
	c.do("GET", "/settings", nil, &st)
	assert.Equal(t, true, st["api_key_set"])
	assert.Equal(t, "…6789", st["api_key_hint"])
	assert.NotContains(t, fmt.Sprint(st), "sk-test")
	assert.Equal(t, upstream.URL+"/v1/messages", st["endpoint"])
	assert.Equal(t, true, st["threads"].(map[string]any)["via_connection"])

	var conv db.Conversation
	require.Equal(t, 201, c.do("POST", "/conversations", map[string]any{}, &conv))
	events := c.stream("/conversations/"+conv.ID+"/messages", map[string]any{"content": "When do we ship?"})
	assert.Equal(t, []string{"user_message", "delta", "assistant_message", "title", "done"}, types(events))

	var cd struct {
		Conversation db.Conversation `json:"conversation"`
		Messages     []db.Message    `json:"messages"`
	}
	c.do("GET", "/conversations/"+conv.ID, nil, &cd)
	assert.Equal(t, "Fake Title", cd.Conversation.Title)
	require.Len(t, cd.Messages, 2)
	assert.Equal(t, 42, cd.Messages[1].InputTokens)

	events = c.stream("/conversations/"+conv.ID+"/regenerate", map[string]any{})
	assert.Equal(t, []string{"delta", "assistant_message", "done"}, types(events))
	var list struct {
		Conversations []map[string]any `json:"conversations"`
	}
	c.do("GET", "/conversations?q=ship", nil, &list)
	require.Len(t, list.Conversations, 1)
}

func TestSendWithoutConnectionKeepsNothing(t *testing.T) {
	store, c := newServer(t, nil)
	require.Equal(t, 200, c.do("PUT", "/settings", map[string]any{
		"api_style": "anthropic", "base_url": "http://127.0.0.1:1", "default_model": "",
	}, nil))
	var conv db.Conversation
	c.do("POST", "/conversations", map[string]any{}, &conv)
	var e map[string]any
	assert.Equal(t, 400, c.do("POST", "/conversations/"+conv.ID+"/messages", map[string]any{"content": "hi"}, &e))
	assert.Contains(t, e["error"], "no default model")
	n, _ := store.CountMessages(conv.ID)
	assert.Zero(t, n, "the user's message is not saved when the turn cannot start")
}

// commitRunner is a thread runner that commits one file per turn.
type commitRunner struct{}

func (commitRunner) RunTurn(ctx context.Context, spec project.TurnSpec, _ agentboot.Prompter, sink func(project.RunnerEvent)) (project.TurnResult, error) {
	sink(project.RunnerEvent{Kind: db.EvAssistant, Content: "Fixing it."})
	if err := os.WriteFile(filepath.Join(spec.Dir, "fix.txt"), []byte("fixed\n"), 0o644); err != nil {
		return project.TurnResult{}, err
	}
	for _, args := range [][]string{{"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "fix"}} {
		if out, err := exec.Command("git", append([]string{"-C", spec.Dir}, args...)...).CombinedOutput(); err != nil {
			return project.TurnResult{}, fmt.Errorf("%v: %s", err, out)
		}
	}
	return project.TurnResult{Text: "Fixed the login bug; committed fix.txt.", InputTokens: 100, OutputTokens: 20}, nil
}

func gitRepo(t *testing.T) string {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

func TestProjectEndToEnd(t *testing.T) {
	fake := &fakeAnthropic{reply: func(body map[string]any) (string, string, any) {
		msgs, _ := body["messages"].([]any)
		last, _ := json.Marshal(msgs[len(msgs)-1])
		switch {
		case strings.Contains(string(last), "tool_result"):
			return "Started a thread for the login bug.", "", nil
		case strings.Contains(string(last), "Coordinator check-in"):
			return "The login bug fix is ready for review.", "", nil
		default:
			return "", "start_thread", map[string]string{"title": "Fix login bug", "task": "Fix the login bug and commit."}
		}
	}}
	upstream := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer upstream.Close()
	store, c := newServer(t, commitRunner{})
	require.Equal(t, 200, c.do("PUT", "/settings", map[string]any{"api_style": "anthropic", "base_url": upstream.URL, "default_model": "fake-model"}, nil))

	repo := gitRepo(t)
	var insp map[string]any
	c.do("POST", "/repos/inspect", map[string]any{"path": repo}, &insp)
	assert.Equal(t, true, insp["ok"])
	assert.Equal(t, "main", insp["branch"])
	c.do("POST", "/repos/inspect", map[string]any{"path": t.TempDir()}, &insp)
	assert.Contains(t, insp["error"], "not a git repository")

	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "Auth", "goal": "Login works", "repos": []string{repo}}, &p))
	require.Len(t, p.Repos, 1)
	assert.Equal(t, "main", p.Repos[0].BaseBranch)

	// Live events.
	resp, err := http.Get(c.base + "/projects/" + p.ID + "/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	sse := bufio.NewScanner(resp.Body)
	require.True(t, sse.Scan()) // ": connected"

	require.Equal(t, 201, c.do("POST", "/projects/"+p.ID+"/messages", map[string]any{"content": "Users can't log in, please fix."}, nil))

	// The coordinator starts a thread; it commits and the coordinator relays the report.
	var msgs struct {
		Messages []db.ProjectMessage `json:"messages"`
	}
	require.Eventually(t, func() bool {
		c.do("GET", "/projects/"+p.ID+"/messages", nil, &msgs)
		for _, m := range msgs.Messages {
			if strings.Contains(m.Content, "ready for review") {
				return true
			}
		}
		return false
	}, 10*time.Second, 50*time.Millisecond)
	kinds := []string{}
	for _, m := range msgs.Messages {
		kinds = append(kinds, m.Kind)
	}
	assert.Equal(t, []string{"text", "thread_started", "text", "thread_update", "text"}, kinds)

	var detail struct {
		Threads []db.Thread `json:"threads"`
		Counts  map[string]int
	}
	c.do("GET", "/projects/"+p.ID, nil, &detail)
	require.Len(t, detail.Threads, 1)
	th := detail.Threads[0]
	assert.Equal(t, db.ThreadReady, th.Status)
	assert.Equal(t, 1, detail.Counts["ready"])

	var gotEvent bool
	deadline := time.After(2 * time.Second)
	lines := make(chan string)
	go func() {
		for sse.Scan() {
			lines <- sse.Text()
		}
	}()
	for !gotEvent {
		select {
		case l := <-lines:
			gotEvent = strings.Contains(l, `"type":"thread"`)
		case <-deadline:
			t.Fatal("no thread event on the stream")
		}
	}

	var diff map[string]any
	require.Equal(t, 200, c.do("GET", "/threads/"+th.ID+"/diff", nil, &diff))
	assert.Contains(t, diff["patch"], "fix.txt")

	var merged db.Thread
	require.Equal(t, 200, c.do("POST", "/threads/"+th.ID+"/merge", nil, &merged))
	assert.Equal(t, db.ThreadResolved, merged.Status)
	assert.FileExists(t, filepath.Join(repo, "fix.txt"))

	// Memory and library.
	require.Equal(t, 200, c.do("PUT", "/projects/"+p.ID+"/memory/auth.md", map[string]any{"content": "Use OAuth."}, nil))
	var mem map[string]any
	c.do("GET", "/projects/"+p.ID+"/memory", nil, &mem)
	assert.Len(t, mem["files"], 2)
	assert.Equal(t, 400, c.do("PUT", "/projects/"+p.ID+"/memory/bad%20name.md", map[string]any{"content": "x"}, nil))

	var usage map[string]any
	c.do("GET", "/projects/"+p.ID+"/usage", nil, &usage)
	assert.Len(t, usage["by_thread"], 1)

	// Pause blocks new work; archive then delete.
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"paused": true}, nil))
	assert.Equal(t, 409, c.do("POST", "/projects/"+p.ID+"/messages", map[string]any{"content": "more"}, nil))
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"paused": false, "archived": true}, nil))
	var list map[string][]map[string]any
	c.do("GET", "/projects?archived=true", nil, &list)
	assert.Len(t, list["projects"], 1)
	require.Equal(t, 200, c.do("DELETE", "/projects/"+p.ID, nil, nil))
	_, err = store.GetThread(th.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
	assert.NoDirExists(t, th.Worktree)
}
