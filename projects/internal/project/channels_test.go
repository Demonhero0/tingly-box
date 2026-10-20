package project

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// sentText is one message the fake sink was asked to deliver.
type sentText struct{ platform, chatID, text string }

type fakeSink struct {
	mu   sync.Mutex
	sent []sentText
	ch   chan sentText
	err  error
}

func newFakeSink() *fakeSink { return &fakeSink{ch: make(chan sentText, 32)} }

func (f *fakeSink) SendChannelText(_ context.Context, platform, chatID, text string) error {
	m := sentText{platform, chatID, text}
	f.mu.Lock()
	f.sent = append(f.sent, m)
	f.mu.Unlock()
	f.ch <- m
	return f.err
}

func (f *fakeSink) next(t *testing.T) sentText {
	t.Helper()
	select {
	case m := <-f.ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was sent to the chat")
		return sentText{}
	}
}

func (f *fakeSink) none(t *testing.T) {
	t.Helper()
	select {
	case m := <-f.ch:
		t.Fatalf("unexpected send: %+v", m)
	case <-time.After(150 * time.Millisecond):
	}
}

// channelFixture is a project with a coordinator whose replies go to chats.
func channelFixture(t *testing.T) (*testFixture, *Service, *fakeSink) {
	t.Helper()
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	})
	src := &fakeLLM{fn: func(req llm.Request) llm.Result {
		return llm.Result{Text: "Echo: " + lastUserText(req)}
	}}
	coord := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, src)
	sink := newFakeSink()
	relay := NewRelay(f.store, sink)
	coord.OnFinal = relay.Message
	f.threads.OnWait = relay.NeedsUser
	return f, &Service{Store: f.store, Layout: f.threads.layout, Hub: f.threads.hub, Threads: f.threads, Coordinator: coord, Relay: relay}, sink
}

func TestChannelMessageReachesTheCoordinatorAndTheReplyComesBack(t *testing.T) {
	f, svc, sink := channelFixture(t)
	b, err := svc.BindChannel("telegram", "42", "Me", f.project.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ChannelTargetProject, b.TargetKind)

	handled, err := svc.PostFromChannel("telegram", "42", "How is it going?")
	require.NoError(t, err)
	require.True(t, handled)

	got := sink.next(t)
	assert.Equal(t, "telegram", got.platform)
	assert.Equal(t, "42", got.chatID)
	assert.Equal(t, "[Demo] Echo: How is it going?", got.text, "replies carry the project name")

	// The question is a user message of the project conversation, marked as
	// coming from the chat, and the web sees the reply too.
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ProjectMessage) bool { return len(m) == 2 })
	assert.Equal(t, db.RoleUser, msgs[0].Role)
	assert.Equal(t, db.SourceIM, msgs[0].Source)
	assert.Equal(t, "telegram", msgs[0].SourcePlatform)
	assert.Equal(t, "42", msgs[0].SourceChatID)
	assert.Equal(t, db.RoleAssistant, msgs[1].Role)
	assert.Empty(t, msgs[1].Source)

	// A message typed on the web is not echoed to the chat.
	_, err = svc.Coordinator.Post(f.project.ID, "From the web", nil)
	require.NoError(t, err)
	assert.Equal(t, "[Demo] Echo: From the web", sink.next(t).text)
	waitMessages(t, f.store, f.project.ID, func(m []db.ProjectMessage) bool { return len(m) == 4 })
	sink.none(t)
}

func TestUnboundChatIsNotHandled(t *testing.T) {
	f, svc, sink := channelFixture(t)
	handled, err := svc.PostFromChannel("telegram", "nobody", "hello")
	require.NoError(t, err)
	assert.False(t, handled)
	sink.none(t)
	msgs, _ := f.store.ListProjectMessages(f.project.ID, 0)
	assert.Empty(t, msgs, "an unbound chat never touches a conversation")
}

func TestBoundPausedProjectReportsTheErrorToTheCaller(t *testing.T) {
	f, svc, _ := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", f.project.ID)
	require.NoError(t, err)
	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetPaused(p, true))

	handled, err := svc.PostFromChannel("telegram", "1", "hi")
	assert.True(t, handled, "a bound chat is never passed on to @cc, even when the project cannot answer")
	assert.ErrorIs(t, err, ErrPaused)
}

func TestRepliesGoToEveryBoundChat(t *testing.T) {
	f, svc, sink := channelFixture(t)
	for _, c := range []string{"phone", "group"} {
		_, err := svc.BindChannel("telegram", c, "", f.project.ID)
		require.NoError(t, err)
	}
	_, err := svc.PostFromChannel("telegram", "phone", "ping")
	require.NoError(t, err)
	first, second := sink.next(t), sink.next(t)
	assert.ElementsMatch(t, []string{"phone", "group"}, []string{first.chatID, second.chatID})
}

func TestMovingAChatChangesWhereRepliesGo(t *testing.T) {
	f, svc, sink := channelFixture(t)
	other := &db.Project{Name: "Other"}
	require.NoError(t, f.store.CreateProject(other))
	require.NoError(t, svc.Layout.Ensure(other.ID))

	_, err := svc.BindChannel("telegram", "1", "", f.project.ID)
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "1", "", other.ID)
	require.NoError(t, err)

	_, err = svc.PostFromChannel("telegram", "1", "hi")
	require.NoError(t, err)
	assert.Equal(t, "[Other] Echo: hi", sink.next(t).text)
	none, _ := f.store.ListProjectMessages(f.project.ID, 0)
	assert.Empty(t, none)
}

func TestBindChannelValidates(t *testing.T) {
	f, svc, _ := channelFixture(t)
	_, err := svc.BindChannel("", "1", "", f.project.ID)
	assert.ErrorIs(t, err, ErrBindingInvalid)
	_, err = svc.BindChannel("telegram", "1", "", "no-such-project")
	assert.ErrorIs(t, err, db.ErrNotFound)

	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetArchived(p, true))
	_, err = svc.BindChannel("telegram", "1", "", f.project.ID)
	assert.ErrorIs(t, err, ErrArchived)
}

func TestThreadReportsAreFoldedToOneLine(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", f.project.ID)
	require.NoError(t, err)

	update := func(kind, text string) db.ProjectMessage {
		return db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleSystem, Kind: db.MsgThreadUpdate, Content: text,
			Payload: `{"thread_id":"t1","title":"Fix login","kind":"` + kind + `","status":"idle"}`}
	}
	svc.Relay.Message(update(ReportFinished, "Fixed the login bug.\n\nDetails:\n- changed a.go\n- changed b.go"))
	assert.Equal(t, `[Demo] Thread "Fix login" finished: Fixed the login bug.`, sink.next(t).text)

	svc.Relay.Message(update(ReportFailed, "exit status 1"))
	assert.Equal(t, `[Demo] Thread "Fix login" failed: exit status 1`, sink.next(t).text)

	svc.Relay.Message(update(ReportNeedsInput, "Keep the legacy flag?"))
	got := sink.next(t).text
	assert.Contains(t, got, `Thread "Fix login" needs you: Keep the legacy flag?`)
	assert.Contains(t, got, "web UI")

	long := strings.Repeat("x", 1000)
	svc.Relay.Message(update(ReportFinished, long))
	assert.Less(t, len([]rune(sink.next(t).text)), 300, "a long report is cut")

	// What the chat or web already shows, and kinds without text, are skipped.
	svc.Relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleUser, Kind: db.MsgText, Content: "mine"})
	svc.Relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleAssistant, Kind: db.MsgThreadStarted, Payload: `{"thread_ids":["a"]}`})
	svc.Relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleAssistant, Kind: db.MsgText, Content: "  "})
	sink.none(t)

	svc.Relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleAssistant, Kind: db.MsgError, Content: "coordinator unavailable"})
	assert.Equal(t, "[Demo] Error: coordinator unavailable", sink.next(t).text)
}

func TestApprovalsAndAsksSendOneLineAndOnlyOnce(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", f.project.ID)
	require.NoError(t, err)
	now := time.Now()
	svc.Relay.now = func() time.Time { return now }

	th := db.Thread{ID: "t1", ProjectID: f.project.ID, Title: "Fix login"}
	svc.Relay.NeedsUser(th)
	assert.Equal(t, `[Demo] Thread "Fix login" needs you. Open Projects in the web UI to answer.`, sink.next(t).text)

	svc.Relay.NeedsUser(th) // the next approval of the same thread, right after
	sink.none(t)

	now = now.Add(relayNeedsUserEvery + time.Second)
	svc.Relay.NeedsUser(th)
	sink.next(t)

	svc.Relay.NeedsUser(db.Thread{ID: "session", Title: "folder"}) // a folder session has no project
	sink.none(t)
}

func TestSinkFailureDoesNotStopLaterMessages(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", f.project.ID)
	require.NoError(t, err)
	sink.err = errors.New("bot offline")
	svc.Relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleAssistant, Kind: db.MsgText, Content: "one"})
	svc.Relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleAssistant, Kind: db.MsgText, Content: "two"})
	assert.Equal(t, "[Demo] one", sink.next(t).text)
	assert.Equal(t, "[Demo] two", sink.next(t).text, "delivered in order, after a failure")
}

func TestWithoutASinkNothingIsRelayed(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { return nil })
	relay := NewRelay(f.store, nil)
	_, err := (&Service{Store: f.store}).BindChannel("telegram", "1", "", f.project.ID)
	require.NoError(t, err)
	relay.Message(db.ProjectMessage{ProjectID: f.project.ID, Role: db.RoleAssistant, Kind: db.MsgText, Content: "hi"})
	relay.NeedsUser(db.Thread{ID: "t", ProjectID: f.project.ID})
	relay.mu.Lock()
	defer relay.mu.Unlock()
	assert.Empty(t, relay.queue)
	assert.False(t, relay.running, "standalone hosts have no IM; the relay does nothing")
}
