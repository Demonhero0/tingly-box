package project

import (
	"context"
	"errors"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Errors for channel bindings.
var (
	ErrBindingInvalid = errors.New("platform, chat_id and one of thread_id or project_id are required")
	// ErrBindFolder is for binding a folder by project id: a folder has no
	// main thread, so the chat is bound to one of its threads.
	ErrBindFolder = errors.New("a folder has no main thread; bind one of its threads (thread_id)")
)

// BindChannel binds an IM chat to a thread; a chat that is already bound
// moves. A project id stands for the project's main thread. title is an
// optional display name for the chat.
func (s *Service) BindChannel(platform, chatID, title, threadID, projectID string) (*db.ChannelBinding, error) {
	platform, chatID = strings.TrimSpace(platform), strings.TrimSpace(chatID)
	threadID, projectID = strings.TrimSpace(threadID), strings.TrimSpace(projectID)
	if platform == "" || chatID == "" || (threadID == "") == (projectID == "") {
		return nil, ErrBindingInvalid
	}
	var t *db.Thread
	if threadID != "" {
		var err error
		if t, err = s.Store.GetThread(threadID); err != nil {
			return nil, err
		}
	} else {
		p, err := s.Store.GetProject(projectID)
		if err != nil {
			return nil, err
		}
		if p.Type == db.ProjectFolder {
			return nil, ErrBindFolder
		}
		if t, err = s.Store.GetThread(p.MainThreadID); err != nil {
			return nil, err
		}
	}
	p, err := s.Store.GetProject(t.ProjectID)
	if err != nil {
		return nil, err
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	b := &db.ChannelBinding{Platform: platform, ChatID: chatID, ChatTitle: strings.TrimSpace(title), ThreadID: t.ID}
	if err := s.Store.UpsertChannelBinding(b); err != nil {
		return nil, err
	}
	return b, nil
}

// PostFromChannel delivers a message from an IM chat to the thread the chat is
// bound to, as a message with source im, exactly as the web would send it
// (Send). handled is false when the chat has no binding, and the caller
// carries on as before. The reply arrives in the chat through the Relay.
func (s *Service) PostFromChannel(platform, chatID, text string) (handled bool, err error) {
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	_, err = s.Send(context.Background(), b.ThreadID, text, nil, Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID})
	return true, err
}
