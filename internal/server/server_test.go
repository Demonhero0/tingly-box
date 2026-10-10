package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-project/internal/db"
)

// fakeAnthropic is a minimal Anthropic Messages endpoint that streams a
// fixed reply and records the requests it got.
type fakeAnthropic struct {
	mu       sync.Mutex
	requests []map[string]any
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

	reply := "Hello from fake"
	if sys, _ := json.Marshal(body["system"]); !bytes.Contains(sys, []byte("BLUEHERON")) && strings.Contains(fmt.Sprint(body["messages"]), "Write a short title") {
		reply = "Fake Title"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ev := func(name, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data) }
	ev("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","model":"fake-model","content":[],"stop_reason":null,"usage":{"input_tokens":42,"output_tokens":0}}}`)
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

func TestProjectChatEndToEnd(t *testing.T) {
	fake := &fakeAnthropic{}
	upstream := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer upstream.Close()

	store, err := db.Open(":memory:")
	require.NoError(t, err)
	defer store.Close()
	srv := httptest.NewServer(New(store))
	defer srv.Close()
	c := client{t: t, base: srv.URL + "/api/v1"}

	// Settings: point at the fake, no default model → first listed model.
	require.Equal(t, 200, c.do("PUT", "/settings", map[string]any{
		"api_style": "anthropic", "base_url": upstream.URL, "api_key": "sk-test-123456789", "default_model": "",
	}, nil))
	var st map[string]any
	c.do("GET", "/settings", nil, &st)
	assert.Equal(t, true, st["api_key_set"])
	assert.Equal(t, "…6789", st["api_key_hint"])
	assert.NotContains(t, fmt.Sprint(st), "sk-test")
	assert.Equal(t, upstream.URL+"/v1/messages", st["endpoint"])

	// Project with instructions and knowledge.
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "Launch", "description": "hidden", "instructions": "Be brief."}, &p))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "plan.md")
	fw.Write([]byte("Codename BLUEHERON ships in March."))
	fw, _ = mw.CreateFormFile("files", "scan.pdf")
	fw.Write([]byte("%PDF-1.4\x00\xff\xfe"))
	mw.Close()
	resp, err := http.Post(c.base+"/projects/"+p.ID+"/knowledge", mw.FormDataContentType(), &buf)
	require.NoError(t, err)
	var up struct {
		Files    []map[string]any `json:"files"`
		Rejected []map[string]any `json:"rejected"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&up))
	resp.Body.Close()
	require.Len(t, up.Files, 1)
	require.Len(t, up.Rejected, 1)
	assert.Contains(t, up.Rejected[0]["reason"], "PDF")

	var detail map[string]any
	c.do("GET", "/projects/"+p.ID, nil, &detail)
	assert.Equal(t, "inline", detail["capacity"].(map[string]any)["mode"])

	// Chat inside the project.
	var conv db.Conversation
	require.Equal(t, 201, c.do("POST", "/conversations", map[string]any{"project_id": p.ID}, &conv))
	events := c.stream("/conversations/"+conv.ID+"/messages", map[string]any{"content": "When do we ship?"})
	assert.Equal(t, []string{"user_message", "delta", "assistant_message", "title", "done"}, types(events))

	var text strings.Builder
	for _, e := range events {
		if e["type"] == "delta" {
			text.WriteString(e["text"].(string))
		}
	}
	assert.Equal(t, "Hello from fake", text.String())

	reqs := fake.chatRequests()
	require.Len(t, reqs, 2, "chat + title")
	sys, _ := json.Marshal(reqs[0]["system"])
	assert.Contains(t, string(sys), "BLUEHERON")
	assert.Contains(t, string(sys), "Be brief.")
	assert.NotContains(t, string(sys), "hidden", "description is never sent")
	assert.Equal(t, "fake-model", reqs[0]["model"])

	var cd struct {
		Conversation db.Conversation `json:"conversation"`
		Project      map[string]any  `json:"project"`
		Messages     []db.Message    `json:"messages"`
	}
	c.do("GET", "/conversations/"+conv.ID, nil, &cd)
	assert.Equal(t, "Fake Title", cd.Conversation.Title)
	assert.Equal(t, "fake-model", cd.Conversation.Model)
	assert.Equal(t, "Launch", cd.Project["name"])
	require.Len(t, cd.Messages, 2)
	assert.Equal(t, 42, cd.Messages[1].InputTokens)
	assert.Equal(t, "end_turn", cd.Messages[1].StopReason)

	// Memory can only be turned off before the first message.
	assert.Equal(t, 409, c.do("PATCH", "/conversations/"+conv.ID, map[string]any{"memory_disabled": true}, nil))

	// Regenerate replaces the reply; no new title.
	events = c.stream("/conversations/"+conv.ID+"/regenerate", map[string]any{})
	assert.Equal(t, []string{"delta", "assistant_message", "done"}, types(events))
	c.do("GET", "/conversations/"+conv.ID, nil, &cd)
	require.Len(t, cd.Messages, 2)

	// Move the chat out of the project and back.
	empty := ""
	require.Equal(t, 200, c.do("PATCH", "/conversations/"+conv.ID, map[string]any{"project_id": empty}, nil))
	var list struct {
		Conversations []map[string]any `json:"conversations"`
	}
	c.do("GET", "/conversations?project_id=none", nil, &list)
	require.Len(t, list.Conversations, 1)
	require.Equal(t, 200, c.do("PATCH", "/conversations/"+conv.ID, map[string]any{"project_id": p.ID}, nil))
	c.do("GET", "/conversations?q=ship", nil, &list)
	require.Len(t, list.Conversations, 1)
	assert.Equal(t, "Launch", list.Conversations[0]["project_name"])

	// Archived projects cannot be deleted until unarchived.
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"archived": true}, nil))
	assert.Equal(t, 409, c.do("DELETE", "/projects/"+p.ID, nil, nil))
	var projects struct {
		Projects []map[string]any `json:"projects"`
	}
	c.do("GET", "/projects?archived=true", nil, &projects)
	require.Len(t, projects.Projects, 1)
	assert.EqualValues(t, 1, projects.Projects[0]["conversation_count"])
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"archived": false}, nil))
	require.Equal(t, 200, c.do("DELETE", "/projects/"+p.ID, nil, nil))
	assert.Equal(t, 404, c.do("GET", "/conversations/"+conv.ID, nil, nil))
}

func TestSendWithoutConnectionKeepsNothing(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	defer store.Close()
	srv := httptest.NewServer(New(store))
	defer srv.Close()
	c := client{t: t, base: srv.URL + "/api/v1"}

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

func TestMemoryRefresh(t *testing.T) {
	fake := &fakeAnthropic{}
	upstream := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer upstream.Close()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	defer store.Close()
	srv := httptest.NewServer(New(store))
	defer srv.Close()
	c := client{t: t, base: srv.URL + "/api/v1"}
	c.do("PUT", "/settings", map[string]any{"api_style": "anthropic", "base_url": upstream.URL, "default_model": "fake-model"}, nil)

	var p db.Project
	c.do("POST", "/projects", map[string]any{"name": "P"}, &p)

	// Nothing to summarize yet: no model call, memory unchanged.
	var got db.Project
	require.Equal(t, 200, c.do("POST", "/projects/"+p.ID+"/memory/refresh", nil, &got))
	assert.Empty(t, got.Memory)
	assert.Empty(t, fake.chatRequests())

	var on, off db.Conversation
	c.do("POST", "/conversations", map[string]any{"project_id": p.ID}, &on)
	c.do("POST", "/conversations", map[string]any{"project_id": p.ID, "memory_disabled": true}, &off)
	c.stream("/conversations/"+on.ID+"/messages", map[string]any{"content": "remember ALPHA"})
	c.stream("/conversations/"+off.ID+"/messages", map[string]any{"content": "secret OMEGA"})

	before := len(fake.chatRequests())
	var raw map[string]any
	status := c.do("POST", "/projects/"+p.ID+"/memory/refresh", nil, &raw)
	require.Equal(t, 200, status, raw)
	b, _ := json.Marshal(raw)
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "Hello from fake", got.Memory)
	require.NotNil(t, got.MemoryUpdatedAt)
	reqs := fake.chatRequests()
	require.Len(t, reqs, before+1)
	prompt := fmt.Sprint(reqs[len(reqs)-1]["messages"])
	assert.Contains(t, prompt, "ALPHA")
	assert.NotContains(t, prompt, "OMEGA", "memory-off chats are never read")

	// The memory now reaches new chats in the project.
	var next db.Conversation
	c.do("POST", "/conversations", map[string]any{"project_id": p.ID}, &next)
	c.stream("/conversations/"+next.ID+"/messages", map[string]any{"content": "hi"})
	reqs = fake.chatRequests()
	var sys string
	for _, r := range reqs {
		if strings.Contains(fmt.Sprint(r["messages"]), "hi") && !strings.Contains(fmt.Sprint(r["messages"]), "Write a short title") {
			b, _ := json.Marshal(r["system"])
			sys = string(b)
		}
	}
	assert.Contains(t, sys, "project_memory")
}
