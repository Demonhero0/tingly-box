package project

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Relay sends a thread's finished replies and folded thread reports to the
// chats bound to that thread (a worker's or folder thread's own turn end
// included), and a descendant's "needs you" to the chats
// bound to it or to any ancestor. Messages go out one at a time, in the order
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
	// threadIDs are the threads whose chats hear it; the first names the
	// project in the prefix.
	threadIDs []string
	text      string
}

// NewRelay builds the relay; sink may be nil, which makes every call a no-op.
func NewRelay(store *db.Store, sink ChannelSink) *Relay {
	return &Relay{store: store, sink: sink, notified: map[string]time.Time{}, now: time.Now}
}

// Message relays a finished event of a main thread. User events, which the
// chat or the web already shows, and kinds without text for a chat are skipped.
func (r *Relay) Message(e db.ThreadEvent) {
	if r == nil || r.sink == nil {
		return
	}
	var text string
	switch e.Kind {
	case db.EvAssistant:
		text = strings.TrimSpace(e.Content)
	case db.EvError:
		if c := strings.TrimSpace(e.Content); c != "" {
			text = "Error: " + c
		}
	case db.EvThreadUpdate:
		text = foldThreadUpdate(e)
	}
	if text != "" {
		r.enqueue([]string{e.ThreadID}, text)
	}
}

// ThreadReply relays the end of a turn of a non-main thread (a worker or a
// folder thread) to the chats bound to that very thread. Chats bound to its
// parent already hear it as the coordinator's thread report, so only this
// thread's own chats are addressed.
func (r *Relay) ThreadReply(rep Report) {
	if r == nil || r.sink == nil {
		return
	}
	text := strings.TrimSpace(rep.Text)
	if text == "" {
		return
	}
	if rep.Kind == ReportFailed {
		text = "Error: " + text
	}
	r.enqueue([]string{rep.Thread.ID}, text)
}

// NeedsUser tells the chats of a thread, and of its ancestors, that it is
// waiting for an approval or an answer. Answering happens in the web UI.
func (r *Relay) NeedsUser(t db.Thread) {
	if r == nil || r.sink == nil {
		return
	}
	r.mu.Lock()
	last, seen := r.notified[t.ID]
	now := r.now()
	if seen && now.Sub(last) < relayNeedsUserEvery {
		r.mu.Unlock()
		return
	}
	for id, at := range r.notified {
		if now.Sub(at) >= relayNeedsUserEvery {
			delete(r.notified, id)
		}
	}
	r.notified[t.ID] = now
	r.mu.Unlock()
	lineage, err := r.store.Lineage(t.ID)
	if err != nil || len(lineage) == 0 {
		return
	}
	ids := make([]string, len(lineage))
	for i, a := range lineage {
		ids[i] = a.ID
	}
	r.enqueue(ids, fmt.Sprintf("%s needs you. Open Projects in the web UI to answer.", threadLabel(t.Title)))
}

func threadLabel(title string) string {
	if title = strings.TrimSpace(title); title == "" {
		return "A thread"
	}
	return fmt.Sprintf("Thread %q", title)
}

// foldThreadUpdate is a thread's report as the one line a chat gets.
func foldThreadUpdate(m db.ThreadEvent) string {
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

func (r *Relay) enqueue(threadIDs []string, text string) {
	r.mu.Lock()
	r.queue = append(r.queue, relayJob{threadIDs: threadIDs, text: text})
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

// deliver sends one message to every chat bound to the job's threads,
// prefixed with the project's name so a chat never forgets where it is.
func (r *Relay) deliver(job relayJob) {
	bindings, err := r.store.ListChannelBindingsOf(job.threadIDs)
	if err != nil || len(bindings) == 0 {
		return
	}
	first, err := r.store.GetThread(job.threadIDs[0])
	if err != nil {
		return
	}
	p, err := r.store.GetProject(first.ProjectID)
	if err != nil {
		return
	}
	text := "[" + p.Name + "] " + job.text
	for _, b := range bindings {
		ctx, cancel := context.WithTimeout(context.Background(), relaySendTimeout)
		if err := r.sink.SendChannelText(ctx, b.Platform, b.ChatID, text); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"platform": b.Platform, "chat_id": b.ChatID, "thread_id": b.ThreadID}).Warn("projects: send to IM chat")
		}
		cancel()
	}
}
