package db

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ---- IM channel bindings ----

// ListChannelBindings returns the bindings of one project, or all of them
// when projectID is empty, newest first.
func (s *Store) ListChannelBindings(projectID string) ([]ChannelBinding, error) {
	out := []ChannelBinding{}
	q := s.db.Order("created_at DESC, rowid DESC")
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	err := q.Find(&out).Error
	return out, err
}

// GetChannelBinding loads the binding of an IM chat.
func (s *Store) GetChannelBinding(platform, chatID string) (*ChannelBinding, error) {
	var b ChannelBinding
	if err := s.db.First(&b, "platform = ? AND chat_id = ?", platform, chatID).Error; err != nil {
		return nil, notFound(err)
	}
	return &b, nil
}

// GetChannelBindingByID loads a binding by its id.
func (s *Store) GetChannelBindingByID(id string) (*ChannelBinding, error) {
	var b ChannelBinding
	if err := s.db.First(&b, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &b, nil
}

// UpsertChannelBinding binds b.Platform/b.ChatID to a project. A chat that
// is already bound moves to the new project (its id and created_at stay, any
// thread focus is dropped, and a known title is kept when b has none). b is
// filled in with the stored row.
func (s *Store) UpsertChannelBinding(b *ChannelBinding) error {
	b.Platform = strings.TrimSpace(b.Platform)
	b.ChatID = strings.TrimSpace(b.ChatID)
	if b.TargetKind == "" {
		b.TargetKind = ChannelTargetProject
	}
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var cur ChannelBinding
		err := tx.First(&cur, "platform = ? AND chat_id = ?", b.Platform, b.ChatID).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			b.ID, b.CreatedAt, b.UpdatedAt = newID(), now, now
			return tx.Create(b).Error
		case err != nil:
			return err
		}
		cur.TargetKind, cur.ProjectID, cur.WorkdirID, cur.FocusThreadID = b.TargetKind, b.ProjectID, b.WorkdirID, nil
		if t := strings.TrimSpace(b.ChatTitle); t != "" {
			cur.ChatTitle = t
		}
		cur.UpdatedAt = now
		if err := tx.Save(&cur).Error; err != nil {
			return err
		}
		*b = cur
		return nil
	})
}

// DeleteChannelBinding unbinds a chat.
func (s *Store) DeleteChannelBinding(id string) error {
	res := s.db.Where("id = ?", id).Delete(&ChannelBinding{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
