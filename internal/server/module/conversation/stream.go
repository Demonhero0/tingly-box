package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/knowledge"
	"github.com/tingly-dev/tingly-project/internal/llm"
	"github.com/tingly-dev/tingly-project/internal/server/module/apierr"
)

// SendMessageRequest sends one user message.
type SendMessageRequest struct {
	Content string `json:"content" binding:"required"`
	// Model overrides the conversation's model for this and later turns.
	Model string `json:"model,omitempty"`
}

// RegenerateRequest replaces the last assistant reply.
type RegenerateRequest struct {
	Model string `json:"model,omitempty"`
}

// Stream event types.
const (
	EventUserMessage      = "user_message"
	EventDelta            = "delta"
	EventAssistantMessage = "assistant_message"
	EventTitle            = "title"
	EventError            = "error"
	EventDone             = "done"
)

// StreamEvent is one `data:` line of a chat stream.
type StreamEvent struct {
	Type    string      `json:"type" enum:"user_message,delta,assistant_message,title,error,done"`
	Message *db.Message `json:"message,omitempty"`
	Text    string      `json:"text,omitempty"`
	Title   string      `json:"title,omitempty"`
	Error   string      `json:"error,omitempty"`
	// Context describes how project knowledge was included (assistant_message only).
	Context *TurnContext `json:"context,omitempty"`
}

// TurnContext reports how a project's knowledge went into one turn.
type TurnContext struct {
	Mode            string `json:"mode"`
	KnowledgeTokens int    `json:"knowledge_tokens"`
	Excerpts        int    `json:"excerpts"`
	MemoryUsed      bool   `json:"memory_used"`
}

func (h *Handler) send(c *gin.Context) {
	var req SendMessageRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		apierr.BadRequest(c, "message is empty")
		return
	}
	h.turn(c, req.Model, func(conv *db.Conversation) (*db.Message, error) {
		m := &db.Message{ConversationID: conv.ID, Role: db.RoleUser, Content: req.Content}
		return m, h.store.CreateMessage(m)
	})
}

func (h *Handler) regenerate(c *gin.Context) {
	var req RegenerateRequest
	if !apierr.Bind(c, &req) {
		return
	}
	h.turn(c, req.Model, func(conv *db.Conversation) (*db.Message, error) {
		msgs, err := h.store.ListMessages(conv.ID)
		if err != nil {
			return nil, err
		}
		var drop []string
		i := len(msgs) - 1
		for ; i >= 0 && msgs[i].Role == db.RoleAssistant; i-- {
			drop = append(drop, msgs[i].ID)
		}
		if i < 0 {
			return nil, errors.New("nothing to regenerate")
		}
		return nil, h.store.DeleteMessages(drop)
	})
}

// turn runs one streaming chat turn. prepare persists whatever starts the
// turn (a new user message, or dropping the old reply) and runs only after
// the connection and model are known to be usable, so a misconfiguration
// answers with a plain JSON error and loses nothing.
func (h *Handler) turn(c *gin.Context, modelOverride string, prepare func(*db.Conversation) (*db.Message, error)) {
	conv, err := h.store.GetConversation(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	if !h.acquire(conv.ID) {
		apierr.Message(c, http.StatusConflict, "a reply is already being generated in this chat")
		return
	}
	defer h.release(conv.ID)

	client, conn, err := h.settings.Client()
	if err != nil {
		apierr.Message(c, http.StatusBadRequest, "model connection: "+err.Error())
		return
	}
	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = conv.Model
	}
	model, err = h.settings.ResolveModel(c.Request.Context(), conn, model)
	if err != nil {
		apierr.Message(c, http.StatusBadRequest, err.Error())
		return
	}

	userMsg, err := prepare(conv)
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}

	history, err := h.store.ListMessages(conv.ID)
	if err != nil {
		apierr.Store(c, err, "messages")
		return
	}
	system, turnCtx, project, err := h.systemPrompt(conv, history)
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}

	sse := newSSE(c)
	if userMsg != nil {
		sse.send(StreamEvent{Type: EventUserMessage, Message: userMsg})
	}

	ctx := c.Request.Context()
	start := time.Now()
	res, streamErr := client.Stream(ctx, llm.Request{
		Model:    model,
		System:   system,
		Messages: toLLM(history),
	}, func(text string) {
		sse.send(StreamEvent{Type: EventDelta, Text: text})
	})

	reply := &db.Message{
		ConversationID: conv.ID,
		Role:           db.RoleAssistant,
		Content:        res.Text,
		Model:          model,
		InputTokens:    res.Usage.InputTokens,
		OutputTokens:   res.Usage.OutputTokens,
		StopReason:     res.StopReason,
	}
	cancelled := ctx.Err() != nil
	switch {
	case cancelled:
		reply.StopReason = "cancelled"
	case streamErr != nil:
		reply.Error = streamErr.Error()
	}
	logrus.WithFields(logrus.Fields{
		"conversation": conv.ID, "model": model, "ms": time.Since(start).Milliseconds(),
		"in": res.Usage.InputTokens, "out": res.Usage.OutputTokens, "err": streamErr,
	}).Info("chat turn")

	// Keep a cancelled reply only if something arrived; an empty one would
	// just be noise in the thread.
	if !(cancelled && reply.Content == "") {
		if err := h.store.CreateMessage(reply); err != nil {
			sse.send(StreamEvent{Type: EventError, Error: err.Error()})
			return
		}
	}
	if conv.Model != model {
		_ = h.store.SetConversationModel(conv.ID, model)
	}
	if cancelled {
		return
	}
	sse.send(StreamEvent{Type: EventAssistantMessage, Message: reply, Context: turnCtx})
	if reply.Error != "" {
		sse.send(StreamEvent{Type: EventError, Error: reply.Error})
	}

	if conv.Title == "" && reply.Error == "" {
		title := h.makeTitle(client, model, history)
		if err := h.store.SetConversationTitle(conv.ID, title); err == nil {
			sse.send(StreamEvent{Type: EventTitle, Title: title})
		}
	}
	if project != nil && !conv.MemoryDisabled && reply.Error == "" && h.memory != nil {
		h.memory.Schedule(project.ID)
	}
	sse.send(StreamEvent{Type: EventDone})
}

// systemPrompt builds the project context for a conversation; chats outside
// a project get none.
func (h *Handler) systemPrompt(conv *db.Conversation, history []db.Message) (string, *TurnContext, *db.Project, error) {
	if conv.ProjectID == "" {
		return "", nil, nil, nil
	}
	p, err := h.store.GetProject(conv.ProjectID)
	if errors.Is(err, db.ErrNotFound) {
		return "", nil, nil, nil
	}
	if err != nil {
		return "", nil, nil, err
	}
	files, err := h.store.ListKnowledgeWithContent(p.ID)
	if err != nil {
		return "", nil, nil, err
	}
	prompt := knowledge.Build(knowledge.PromptInput{
		Project: p,
		Files:   files,
		Query:   retrievalQuery(history),
		Memory:  !conv.MemoryDisabled,
	})
	return prompt.System, &TurnContext{
		Mode:            prompt.Mode,
		KnowledgeTokens: prompt.KnowledgeTokens,
		Excerpts:        prompt.Excerpts,
		MemoryUsed:      !conv.MemoryDisabled && strings.TrimSpace(p.Memory) != "",
	}, p, nil
}

// retrievalQuery is the last two user messages, the best signal for what
// knowledge the next reply needs.
func retrievalQuery(history []db.Message) string {
	var parts []string
	for i := len(history) - 1; i >= 0 && len(parts) < 2; i-- {
		if history[i].Role == db.RoleUser {
			parts = append(parts, history[i].Content)
		}
	}
	return strings.Join(parts, "\n")
}

// toLLM converts stored history to model messages: empty replies (failed
// turns) are skipped and consecutive same-role turns are merged, since the
// Anthropic API requires strict alternation.
func toLLM(history []db.Message) []llm.Message {
	var out []llm.Message
	for _, m := range history {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Content += "\n\n" + m.Content
			continue
		}
		out = append(out, llm.Message{Role: m.Role, Content: m.Content})
	}
	// A reply must follow a user turn.
	for len(out) > 0 && out[0].Role != db.RoleUser {
		out = out[1:]
	}
	return out
}

const titlePrompt = `Write a short title (at most 6 words) for a chat that starts with the message below. Use the same language as the message. Reply with the title only: no quotes, no punctuation at the end.

<message>
%s
</message>`

// makeTitle asks the model for a title, falling back to the first words of
// the first user message.
func (h *Handler) makeTitle(client llm.Client, model string, history []db.Message) string {
	first := ""
	for _, m := range history {
		if m.Role == db.RoleUser {
			first = m.Content
			break
		}
	}
	fallback := truncateRunes(strings.Join(strings.Fields(first), " "), 50)
	if fallback == "" {
		fallback = "New chat"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := llm.Complete(ctx, client, llm.Request{
		Model:     model,
		Messages:  []llm.Message{{Role: db.RoleUser, Content: fmt.Sprintf(titlePrompt, truncateRunes(first, 2000))}},
		MaxTokens: 32,
	})
	if err != nil {
		return fallback
	}
	title := strings.TrimSpace(strings.SplitN(strings.TrimSpace(res.Text), "\n", 2)[0])
	title = strings.Trim(title, "\"'“”‘’「」#* ")
	title = strings.TrimRight(title, ".。")
	if title == "" {
		return fallback
	}
	return truncateRunes(title, 80)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// sse writes server-sent events, one JSON object per data line.
type sse struct {
	c *gin.Context
}

func newSSE(c *gin.Context) *sse {
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()
	return &sse{c: c}
}

func (s *sse) send(ev StreamEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(s.c.Writer, "data: %s\n\n", b); err != nil {
		return
	}
	s.c.Writer.Flush()
}
