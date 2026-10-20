package db

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// ---- Projects ----

// ListProjects returns active or archived projects whose name or goal match q.
func (s *Store) ListProjects(q string, archived bool) ([]Project, error) {
	var out []Project
	tx := s.db.Model(&Project{})
	if archived {
		tx = tx.Where("archived_at IS NOT NULL")
	} else {
		tx = tx.Where("archived_at IS NULL")
	}
	if q = strings.TrimSpace(q); q != "" {
		p := likePattern(q)
		tx = tx.Where(`name LIKE ? ESCAPE '\' OR goal LIKE ? ESCAPE '\'`, p, p)
	}
	err := tx.Order("starred DESC, updated_at DESC").Find(&out).Error
	return out, err
}

// CreateProject inserts a project.
func (s *Store) CreateProject(p *Project) error {
	p.ID = newID()
	return s.db.Create(p).Error
}

// GetProject loads a project.
func (s *Store) GetProject(id string) (*Project, error) {
	var p Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

// SaveProject writes all fields of p.
func (s *Store) SaveProject(p *Project) error {
	return s.db.Save(p).Error
}

// TouchProject bumps a project's UpdatedAt (recent activity).
func (s *Store) TouchProject(id string) error {
	return s.db.Model(&Project{}).Where("id = ?", id).UpdateColumn("updated_at", time.Now()).Error
}

// DeleteProject removes a project with its conversation, threads and events.
func (s *Store) DeleteProject(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		threadIDs := tx.Model(&Thread{}).Select("id").Where("project_id = ?", id)
		if err := tx.Where("thread_id IN (?)", threadIDs).Delete(&ThreadEvent{}).Error; err != nil {
			return err
		}
		for _, m := range []any{&Thread{}, &ProjectMessage{}, &ChannelBinding{}} {
			if err := tx.Where("project_id = ?", id).Delete(m).Error; err != nil {
				return err
			}
		}
		res := tx.Where("id = ?", id).Delete(&Project{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ThreadCounts maps project ID → thread status → count.
func (s *Store) ThreadCounts() (map[string]map[string]int, error) {
	var rows []struct {
		ProjectID string
		Status    string
		N         int
	}
	if err := s.db.Model(&Thread{}).Select("project_id, status, COUNT(*) AS n").Group("project_id, status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]map[string]int{}
	for _, r := range rows {
		if out[r.ProjectID] == nil {
			out[r.ProjectID] = map[string]int{}
		}
		out[r.ProjectID][r.Status] = r.N
	}
	return out, nil
}

// ---- Project conversation ----

// AddProjectMessage appends to a project's conversation.
func (s *Store) AddProjectMessage(m *ProjectMessage) error {
	m.ID = newID()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return s.db.Create(m).Error
}

// UpdateProjectMessage rewrites a message's content and payload.
func (s *Store) UpdateProjectMessage(m *ProjectMessage) error {
	return s.db.Model(&ProjectMessage{}).Where("id = ?", m.ID).
		UpdateColumns(map[string]any{"content": m.Content, "payload": m.Payload, "kind": m.Kind}).Error
}

// ListProjectMessages returns the latest limit messages of a project's
// conversation in chronological order (all when limit <= 0).
func (s *Store) ListProjectMessages(projectID string, limit int) ([]ProjectMessage, error) {
	var out []ProjectMessage
	tx := s.db.Where("project_id = ?", projectID).Order("created_at DESC, rowid DESC")
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	if err := tx.Find(&out).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ---- Threads ----

// CreateThread inserts a thread.
func (s *Store) CreateThread(t *Thread) error {
	t.ID = newID()
	return s.db.Create(t).Error
}

// GetThread loads a thread.
func (s *Store) GetThread(id string) (*Thread, error) {
	var t Thread
	if err := s.db.First(&t, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

// SaveThread writes all fields of t.
func (s *Store) SaveThread(t *Thread) error {
	return s.db.Save(t).Error
}

// ListThreads returns a project's threads, most recently updated first.
func (s *Store) ListThreads(projectID string) ([]Thread, error) {
	var out []Thread
	err := s.db.Where("project_id = ?", projectID).Order("updated_at DESC").Find(&out).Error
	return out, err
}

// ListThreadsByStatus returns threads in any of the given statuses across
// all projects.
func (s *Store) ListThreadsByStatus(statuses ...string) ([]Thread, error) {
	var out []Thread
	err := s.db.Where("status IN ?", statuses).Find(&out).Error
	return out, err
}

// ---- Thread events ----

// AddThreadEvent appends to a thread's transcript.
func (s *Store) AddThreadEvent(e *ThreadEvent) error {
	e.ID = newID()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return s.db.Create(e).Error
}

// UpdateThreadEvent rewrites an event's content and payload (e.g. an
// approval once answered).
func (s *Store) UpdateThreadEvent(e *ThreadEvent) error {
	return s.db.Model(&ThreadEvent{}).Where("id = ?", e.ID).
		UpdateColumns(map[string]any{"content": e.Content, "payload": e.Payload}).Error
}

// ListThreadEvents returns a thread's transcript in order.
func (s *Store) ListThreadEvents(threadID string) ([]ThreadEvent, error) {
	var out []ThreadEvent
	err := s.db.Where("thread_id = ?", threadID).Order("created_at ASC, rowid ASC").Find(&out).Error
	return out, err
}

// ProjectUsage sums thread token usage by model for a project.
func (s *Store) ProjectUsage(projectID string) ([]ModelUsage, error) {
	var out []ModelUsage
	err := s.db.Model(&Thread{}).
		Select("model, COUNT(*) AS threads, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens").
		Where("project_id = ?", projectID).Group("model").Scan(&out).Error
	return out, err
}

// ModelUsage is token use aggregated per model.
type ModelUsage struct {
	Model        string `json:"model"`
	Threads      int    `json:"threads"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}
