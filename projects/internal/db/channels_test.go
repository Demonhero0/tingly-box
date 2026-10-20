package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestChannelBindingUpsertMovesAChat(t *testing.T) {
	s := newStore(t)
	a, b := &Project{Name: "A"}, &Project{Name: "B"}
	require.NoError(t, s.CreateProject(a))
	require.NoError(t, s.CreateProject(b))

	first := &ChannelBinding{Platform: "telegram", ChatID: "42", ChatTitle: "Me", ProjectID: a.ID}
	require.NoError(t, s.UpsertChannelBinding(first))
	assert.NotEmpty(t, first.ID)
	assert.Equal(t, ChannelTargetProject, first.TargetKind)

	// Same chat again: it moves, keeping its id, and keeps its title when the
	// new request has none.
	moved := &ChannelBinding{Platform: "telegram", ChatID: "42", ProjectID: b.ID}
	require.NoError(t, s.UpsertChannelBinding(moved))
	assert.Equal(t, first.ID, moved.ID)
	assert.Equal(t, b.ID, moved.ProjectID)
	assert.Equal(t, "Me", moved.ChatTitle)

	// Another platform's chat with the same id is a different chat.
	require.NoError(t, s.UpsertChannelBinding(&ChannelBinding{Platform: "feishu", ChatID: "42", ProjectID: a.ID}))

	all, err := s.ListChannelBindings("")
	require.NoError(t, err)
	assert.Len(t, all, 2)
	onB, err := s.ListChannelBindings(b.ID)
	require.NoError(t, err)
	require.Len(t, onB, 1)
	assert.Equal(t, "telegram", onB[0].Platform)
	onA, err := s.ListChannelBindings(a.ID)
	require.NoError(t, err)
	assert.Len(t, onA, 1, "moving a chat away from A leaves only the feishu chat there")

	got, err := s.GetChannelBinding("telegram", "42")
	require.NoError(t, err)
	assert.Equal(t, b.ID, got.ProjectID)
	_, err = s.GetChannelBinding("telegram", "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteChannelBindingAndProject(t *testing.T) {
	s := newStore(t)
	p := &Project{Name: "P"}
	require.NoError(t, s.CreateProject(p))
	b := &ChannelBinding{Platform: "telegram", ChatID: "1", ProjectID: p.ID}
	require.NoError(t, s.UpsertChannelBinding(b))

	require.NoError(t, s.DeleteChannelBinding(b.ID))
	assert.ErrorIs(t, s.DeleteChannelBinding(b.ID), ErrNotFound)

	require.NoError(t, s.UpsertChannelBinding(&ChannelBinding{Platform: "telegram", ChatID: "2", ProjectID: p.ID}))
	require.NoError(t, s.DeleteProject(p.ID))
	left, err := s.ListChannelBindings("")
	require.NoError(t, err)
	assert.Empty(t, left, "deleting a project unbinds its chats")
}

func TestProjectMessageKeepsItsSource(t *testing.T) {
	s := newStore(t)
	p := &Project{Name: "P"}
	require.NoError(t, s.CreateProject(p))
	require.NoError(t, s.AddProjectMessage(&ProjectMessage{ProjectID: p.ID, Role: RoleUser, Kind: MsgText, Content: "web"}))
	require.NoError(t, s.AddProjectMessage(&ProjectMessage{ProjectID: p.ID, Role: RoleUser, Kind: MsgText, Content: "im",
		Source: SourceIM, SourcePlatform: "telegram", SourceChatID: "42"}))
	msgs, err := s.ListProjectMessages(p.ID, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Empty(t, msgs[0].Source, "empty source is the web")
	assert.Equal(t, SourceIM, msgs[1].Source)
	assert.Equal(t, "telegram", msgs[1].SourcePlatform)
	assert.Equal(t, "42", msgs[1].SourceChatID)
}
