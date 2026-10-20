package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ChannelSink is an optional Host capability: the host can deliver text to an
// IM chat. Projects type-asserts it, so the standalone binary, which has no
// IM, keeps compiling and simply never sends. See .design/projects-im.md.
type ChannelSink interface {
	// SendChannelText delivers text to the chat identified by platform and
	// chatID, as the host's bot for that chat.
	SendChannelText(ctx context.Context, platform, chatID, text string) error
}

// Limits for what the relay says in a chat.
const (
	// relayLineMax caps a folded thread report, which is one line in a chat.
	relayLineMax = 240
	// relayNeedsUserEvery is how often a thread may ping the same chats for
	// "needs you"; a run of approvals in one thread is one nudge.
	relayNeedsUserEvery = 5 * time.Minute
	relaySendTimeout    = 30 * time.Second
)

// Relay sends the coordinator's finished replies and folded thread reports to
// the chats bound to a project. Messages go out one at a time, in the order
// they finished.
type Relay struct {
	store *db.Store
	sink  ChannelSink

	mu       sync.Mutex
	queue    []relayJob
	running  bool
	notified map[string]time.Time
	now      func() time.Time
}

type relayJob struct {
	projectID string
	text      string
}

// NewRelay builds the relay; sink may be nil, which makes every call a no-op.
func NewRelay(store *db.Store, sink ChannelSink) *Relay {
	return &Relay{store: store, sink: sink, notified: map[string]time.Time{}, now: time.Now}
}

// Message relays a finished project message. User messages, which the chat or
// the web already shows, and message kinds without text for a chat are skipped.
func (r *Relay) Message(m db.ProjectMessage) {
	if r == nil || r.sink == nil || m.Role == db.RoleUser {
		return
	}
	var text string
	switch m.Kind {
	case db.MsgText:
		text = strings.TrimSpace(m.Content)
	case db.MsgError:
		if c := strings.TrimSpace(m.Content); c != "" {
			text = "Error: " + c
		}
	case db.MsgThreadUpdate:
		text = foldThreadUpdate(m)
	}
	if text != "" {
		r.enqueue(m.ProjectID, text)
	}
}

// NeedsUser tells the chats of a project that one of its threads is waiting
// for an approval or an answer. Answering happens in the web UI.
func (r *Relay) NeedsUser(t db.Thread) {
	if r == nil || r.sink == nil || t.ProjectID == "" {
		return
	}
	r.mu.Lock()
	last, seen := r.notified[t.ID]
	now := r.now()
	if seen && now.Sub(last) < relayNeedsUserEvery {
		r.mu.Unlock()
		return
	}
	r.notified[t.ID] = now
	r.mu.Unlock()
	r.enqueue(t.ProjectID, fmt.Sprintf("%s needs you. Open Projects in the web UI to answer.", threadLabel(t.Title)))
}

func threadLabel(title string) string {
	if title = strings.TrimSpace(title); title == "" {
		return "A thread"
	}
	return fmt.Sprintf("Thread %q", title)
}

// foldThreadUpdate is a thread's report as the one line a chat gets.
func foldThreadUpdate(m db.ProjectMessage) string {
	var p struct {
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	_ = json.Unmarshal([]byte(m.Payload), &p)
	label := threadLabel(p.Title)
	body := oneLine(m.Content)
	switch p.Kind {
	case ReportFailed:
		return join(label+" failed", body)
	case ReportNeedsInput:
		return join(label+" needs you", body) + ". Open Projects in the web UI to answer."
	default:
		return join(label+" finished", body)
	}
}

func join(head, body string) string {
	if body == "" {
		return head
	}
	return head + ": " + body
}

// oneLine is the first non-empty line of s, cut to relayLineMax runes.
func oneLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > relayLineMax {
			line = string(r[:relayLineMax]) + "..."
		}
		return line
	}
	return ""
}

func (r *Relay) enqueue(projectID, text string) {
	r.mu.Lock()
	r.queue = append(r.queue, relayJob{projectID: projectID, text: text})
	start := !r.running
	r.running = true
	r.mu.Unlock()
	if start {
		go r.drain()
	}
}

func (r *Relay) drain() {
	for {
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.running = false
			r.mu.Unlock()
			return
		}
		job := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()
		r.deliver(job)
	}
}

// deliver sends one message to every chat bound to the project, prefixed with
// the project's name so a chat never forgets which project it is talking to.
func (r *Relay) deliver(job relayJob) {
	bindings, err := r.store.ListChannelBindings(job.projectID)
	if err != nil || len(bindings) == 0 {
		return
	}
	p, err := r.store.GetProject(job.projectID)
	if err != nil {
		return
	}
	text := "[" + p.Name + "] " + job.text
	for _, b := range bindings {
		if b.TargetKind != db.ChannelTargetProject {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), relaySendTimeout)
		if err := r.sink.SendChannelText(ctx, b.Platform, b.ChatID, text); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"platform": b.Platform, "chat_id": b.ChatID, "project_id": job.projectID}).Warn("projects: send to IM chat")
		}
		cancel()
	}
}

// Errors for channel bindings.
var (
	ErrBindingInvalid = errors.New("platform, chat_id and project_id are required")
	// ErrUnsupportedTarget is for a binding target Projects does not serve yet.
	ErrUnsupportedTarget = errors.New("only a project can be bound for now")
)

// BindChannel binds an IM chat to a project; a chat that is already bound
// moves. title is an optional display name for the chat.
func (s *Service) BindChannel(platform, chatID, title, projectID string) (*db.ChannelBinding, error) {
	platform, chatID, projectID = strings.TrimSpace(platform), strings.TrimSpace(chatID), strings.TrimSpace(projectID)
	if platform == "" || chatID == "" || projectID == "" {
		return nil, ErrBindingInvalid
	}
	p, err := s.Store.GetProject(projectID)
	if err != nil {
		return nil, err
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	b := &db.ChannelBinding{Platform: platform, ChatID: chatID, ChatTitle: strings.TrimSpace(title), TargetKind: db.ChannelTargetProject, ProjectID: projectID}
	if err := s.Store.UpsertChannelBinding(b); err != nil {
		return nil, err
	}
	return b, nil
}

// PostFromChannel delivers a message from an IM chat to the project the chat
// is bound to, as a user message with source im, and runs a coordinator turn
// on it like the web does. handled is false when the chat has no binding (or
// one this version does not serve), and the caller carries on as before. The
// reply arrives in the chat through the Relay.
func (s *Service) PostFromChannel(platform, chatID, text string) (handled bool, err error) {
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if b.TargetKind != db.ChannelTargetProject {
		return false, nil
	}
	_, err = s.Coordinator.PostFrom(b.ProjectID, text, nil, Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID})
	return true, err
}
