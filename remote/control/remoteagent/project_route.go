package remoteagent

import (
	"context"

	"github.com/sirupsen/logrus"
)

// ProjectRouter is the optional seam through which a chat that was bound to a
// Projects project talks to that project's coordinator instead of @cc / @tb.
// The host implements it (see internal/server/module/projects); a BotHandler
// without one — the standalone CLI, the test harness — behaves exactly as it
// always did. See .design/projects-im.md.
type ProjectRouter interface {
	// Bound reports whether the chat is bound to a project.
	Bound(platform, chatID string) bool
	// RouteInbound hands a chat's text to the project it is bound to.
	// handled is false when the chat is not bound (the caller carries on with
	// the usual routing). When handled is true the project owns the message:
	// its reply comes back through the host's outbound path, and err, if set,
	// is why the message could not be delivered (for example a paused
	// project) and is shown to the chat.
	RouteInbound(ctx context.Context, in ProjectInbound) (handled bool, err error)
}

// ProjectInbound is one message from a chat.
type ProjectInbound struct {
	Platform string
	BotUUID  string
	ChatID   string
	SenderID string
	Text     string
}

// SetProjectRouter plugs in the router; nil turns it off.
func (h *BotHandler) SetProjectRouter(r ProjectRouter) { h.projectRouter = r }

// routeToProject sends a chat's text to its bound project. It reports whether
// the message was taken; only a chat with a binding is ever taken, so every
// other chat keeps the @cc / @tb routing untouched.
func (h *BotHandler) routeToProject(hCtx HandlerContext, text string) bool {
	if h.projectRouter == nil {
		return false
	}
	handled, err := h.projectRouter.RouteInbound(h.ctx, ProjectInbound{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
		SenderID: hCtx.SenderID,
		Text:     text,
	})
	if !handled {
		return false
	}
	if err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to deliver chat message to its project")
		h.SendText(hCtx, "Could not send this to the project: "+err.Error())
	}
	return true
}

// chatBoundToProject reports whether the chat talks to a project.
func (h *BotHandler) chatBoundToProject(hCtx HandlerContext) bool {
	return h.projectRouter != nil && h.projectRouter.Bound(string(hCtx.Platform), hCtx.ChatID)
}
