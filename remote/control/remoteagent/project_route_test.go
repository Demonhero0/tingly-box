package remoteagent_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
)

// bootHelperNotStarted is bootHelper without starting the manager, so a test
// can plug in a router before messages flow.
func bootHelperNotStarted(t *testing.T) (*testenv.TestEnv, *remoteagent.TestHarness, *testenv.User) {
	t.Helper()
	env := testenv.NewTestEnv(t)
	rp := false
	setting := bot.BotSetting{
		UUID: env.BotUUID(), Name: "tingly-test", Platform: "tingly", AuthType: "none",
		Auth: map[string]string{}, Enabled: true, RequirePairing: &rp,
	}
	harness := remoteagent.BootForTest(t, env.Manager(), setting)
	return env, harness, env.NewUser("alice")
}

// fakeRouter binds the chats in bound and records what it was handed.
type fakeRouter struct {
	mu    sync.Mutex
	bound map[string]bool
	err   error
	got   []remoteagent.ProjectInbound
}

func (f *fakeRouter) bind(chatID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bound[chatID] = true
}

func (f *fakeRouter) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeRouter) Bound(_, chatID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bound[chatID]
}

func (f *fakeRouter) RouteInbound(_ context.Context, in remoteagent.ProjectInbound) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.bound[in.ChatID] {
		return false, nil
	}
	f.got = append(f.got, in)
	return true, f.err
}

func (f *fakeRouter) inbound() []remoteagent.ProjectInbound {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]remoteagent.ProjectInbound(nil), f.got...)
}

// A chat bound to a project talks to the project: the text (even "@cc ...")
// goes to the router, and @cc / @tb never see it.
func Test_BoundChatGoesToItsProject(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	chat.SendText("@cc fix the login bug")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	in := router.inbound()[0]
	assert.Equal(t, "@cc fix the login bug", in.Text)
	assert.Equal(t, "tingly", in.Platform)
	assert.Equal(t, chat.ChatID, in.ChatID)
	assert.Equal(t, harness.Setting.UUID, in.BotUUID)

	// Delivered: the reply is the project's job, so the handler says nothing.
	chat.ExpectNoEvent(700*time.Millisecond, tingly.EventSend, tingly.EventEdit)

	// A project that cannot take the message says why, in the chat.
	router.setErr(errors.New("the project is paused; resume it first"))
	chat.SendText("hello?")
	chat.WaitText(3*time.Second).AssertContains(t, "Could not send this to the project").AssertContains(t, "paused")

	// Commands still work in a bound chat.
	chat.Drain()
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "Your ID:")
	assert.Len(t, router.inbound(), 2, "commands are not sent to the project")
}

// A chat without a binding behaves exactly as before: the router is asked,
// declines, and the usual routing answers.
func Test_UnboundChatKeepsTheUsualRouting(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "@cc")
	chat.SendText("hello there")
	chat.WaitAnySend(5 * time.Second)
	assert.Empty(t, router.inbound(), "an unbound chat is never handed to a project")
}

// Without a router (the CLI, the harness) nothing changes at all.
func Test_NoRouterNoChange(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "@cc")
}
