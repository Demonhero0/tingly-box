package project

import (
	"context"
	"time"
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
