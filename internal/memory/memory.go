// Package memory maintains per-project memory: a short, model-written summary
// of what the user's chats in a project established, carried into every new
// chat in that project. Chats with memory turned off are never read.
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/knowledge"
	"github.com/tingly-dev/tingly-project/internal/llm"
)

// ClientSource provides the model client and model memory updates run on.
type ClientSource interface {
	ClientAndModel(ctx context.Context) (llm.Client, string, error)
}

const (
	// debounce waits for a burst of turns to settle before summarizing.
	debounce = 30 * time.Second
	// maxConversations bounds how many chats one refresh reads.
	maxConversations = 8
	// transcriptBudget bounds the transcript tokens sent per refresh.
	transcriptBudget = 24_000
	maxMemoryTokens  = 1024
)

// Updater refreshes project memory, on demand or debounced after chat turns.
type Updater struct {
	store  *db.Store
	source ClientSource

	mu      sync.Mutex
	timers  map[string]*time.Timer
	running map[string]*sync.Mutex
}

// NewUpdater builds an Updater.
func NewUpdater(store *db.Store, source ClientSource) *Updater {
	return &Updater{store: store, source: source, timers: map[string]*time.Timer{}, running: map[string]*sync.Mutex{}}
}

// Schedule queues a refresh for projectID after the debounce delay,
// restarting the delay if one is already queued.
func (u *Updater) Schedule(projectID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if t, ok := u.timers[projectID]; ok {
		t.Stop()
	}
	u.timers[projectID] = time.AfterFunc(debounce, func() {
		u.mu.Lock()
		delete(u.timers, projectID)
		u.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := u.Refresh(ctx, projectID); err != nil {
			logrus.WithError(err).WithField("project", projectID).Warn("memory refresh failed")
		}
	})
}

func (u *Updater) lock(projectID string) func() {
	u.mu.Lock()
	m, ok := u.running[projectID]
	if !ok {
		m = &sync.Mutex{}
		u.running[projectID] = m
	}
	u.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// Refresh folds the project's chats since the last refresh into its memory
// and returns the updated project. With nothing new it returns the project
// unchanged.
func (u *Updater) Refresh(ctx context.Context, projectID string) (*db.Project, error) {
	defer u.lock(projectID)()

	p, err := u.store.GetProject(projectID)
	if err != nil {
		return nil, err
	}
	convs, err := u.store.MemoryConversations(projectID, p.MemoryUpdatedAt, maxConversations)
	if err != nil {
		return nil, err
	}
	transcript, err := u.transcript(convs)
	if err != nil {
		return nil, err
	}
	if transcript == "" {
		return p, nil
	}

	client, model, err := u.source.ClientAndModel(ctx)
	if err != nil {
		return nil, err
	}
	res, err := llm.Complete(ctx, client, llm.Request{
		Model:     model,
		System:    systemPrompt,
		Messages:  []llm.Message{{Role: "user", Content: userPrompt(p.Memory, transcript)}},
		MaxTokens: maxMemoryTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("update memory: %w", err)
	}
	memory := strings.TrimSpace(res.Text)
	now := time.Now()
	if err := u.store.SetProjectMemory(p.ID, memory, now); err != nil {
		return nil, err
	}
	p.Memory, p.MemoryUpdatedAt = memory, &now
	return p, nil
}

// transcript renders conversations oldest first, newest messages kept when
// the budget runs out.
func (u *Updater) transcript(convs []db.Conversation) (string, error) {
	var parts []string
	used := 0
	for _, c := range convs { // newest first
		msgs, err := u.store.ListMessages(c.ID)
		if err != nil {
			return "", err
		}
		var lines []string
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if m.Content == "" {
				continue
			}
			line := fmt.Sprintf("%s: %s", strings.ToUpper(m.Role[:1])+m.Role[1:], m.Content)
			t := knowledge.EstimateTokens(line)
			if used+t > transcriptBudget {
				break
			}
			used += t
			lines = append([]string{line}, lines...)
		}
		if len(lines) == 0 {
			break
		}
		title := c.Title
		if title == "" {
			title = "Untitled"
		}
		parts = append([]string{fmt.Sprintf("<chat title=%q>\n%s\n</chat>", title, strings.Join(lines, "\n\n"))}, parts...)
		if used >= transcriptBudget {
			break
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

const systemPrompt = `You maintain the memory of a project workspace. The memory is given to the assistant at the start of every new chat in the project, so it must hold what would help in future chats: the user's goals for the project, preferences about tone and format, decisions made, important facts and context, and open threads.

Rules:
- Merge the new chats into the existing memory. Keep what is still true, update what changed, drop what is obsolete or one-off.
- Write concise Markdown bullet points grouped under short headings. Stay under 250 words.
- Do not record secrets, credentials or sensitive personal data.
- Write in the language the user mostly uses.
- Output only the memory, with no preamble.`

func userPrompt(existing, transcript string) string {
	if strings.TrimSpace(existing) == "" {
		existing = "(empty)"
	}
	return fmt.Sprintf("<existing_memory>\n%s\n</existing_memory>\n\n<new_chats>\n%s\n</new_chats>", existing, transcript)
}
