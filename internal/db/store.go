package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound is returned when a lookup by ID finds nothing.
var ErrNotFound = errors.New("not found")

// Store wraps the gorm handle with the queries the modules need.
type Store struct {
	db *gorm.DB
}

// Open opens (creating if needed) the SQLite database at path and migrates
// the schema. Pass ":memory:" for an in-memory database in tests.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		path += "?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL"
	}
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if path == ":memory:" {
		// Each new connection to :memory: is a fresh database; pin to one.
		sqlDB, err := g.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
	}
	if err := g.AutoMigrate(&Project{}, &KnowledgeFile{}, &Conversation{}, &Message{}, &Setting{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: g}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func newID() string { return uuid.NewString() }

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func likePattern(q string) string {
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return "%" + q + "%"
}

// ---- Projects ----

// ProjectStats are aggregates shown on project cards.
type ProjectStats struct {
	ConversationCount int
	KnowledgeTokens   int
	KnowledgeCount    int
	LastActivityAt    time.Time
}

// ListProjects returns projects matching q (name/description), archived or
// active, starred first then most recently active.
func (s *Store) ListProjects(q string, archived bool) ([]Project, map[string]ProjectStats, error) {
	var projects []Project
	tx := s.db.Model(&Project{})
	if archived {
		tx = tx.Where("archived_at IS NOT NULL")
	} else {
		tx = tx.Where("archived_at IS NULL")
	}
	if q = strings.TrimSpace(q); q != "" {
		p := likePattern(q)
		tx = tx.Where(`name LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\'`, p, p)
	}
	if err := tx.Find(&projects).Error; err != nil {
		return nil, nil, err
	}
	stats := make(map[string]ProjectStats, len(projects))
	for _, p := range projects {
		stats[p.ID] = ProjectStats{LastActivityAt: p.UpdatedAt}
	}

	type convAgg struct {
		ProjectID string
		N         int
		Last      string
	}
	var convs []convAgg
	if err := s.db.Model(&Conversation{}).
		Select("project_id, COUNT(*) AS n, MAX(updated_at) AS last").
		Where("project_id <> ''").Group("project_id").Scan(&convs).Error; err != nil {
		return nil, nil, err
	}
	for _, a := range convs {
		st, ok := stats[a.ProjectID]
		if !ok {
			continue
		}
		st.ConversationCount = a.N
		if t, ok := parseSQLiteTime(a.Last); ok && t.After(st.LastActivityAt) {
			st.LastActivityAt = t
		}
		stats[a.ProjectID] = st
	}

	type kAgg struct {
		ProjectID string
		N         int
		Tokens    int
	}
	var ks []kAgg
	if err := s.db.Model(&KnowledgeFile{}).
		Select("project_id, COUNT(*) AS n, COALESCE(SUM(tokens),0) AS tokens").
		Group("project_id").Scan(&ks).Error; err != nil {
		return nil, nil, err
	}
	for _, a := range ks {
		st, ok := stats[a.ProjectID]
		if !ok {
			continue
		}
		st.KnowledgeCount = a.N
		st.KnowledgeTokens = a.Tokens
		stats[a.ProjectID] = st
	}
	return projects, stats, nil
}

// parseSQLiteTime parses the text form SQLite returns for MAX() over a
// datetime column.
func parseSQLiteTime(s string) (time.Time, bool) {
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// CreateProject inserts a new project.
func (s *Store) CreateProject(p *Project) error {
	p.ID = newID()
	return s.db.Create(p).Error
}

// GetProject loads a project by ID.
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

// TouchProject bumps a project's UpdatedAt.
func (s *Store) TouchProject(id string) error {
	return s.db.Model(&Project{}).Where("id = ?", id).Update("updated_at", time.Now()).Error
}

// DeleteProject removes a project with its knowledge, conversations and
// messages.
func (s *Store) DeleteProject(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		convIDs := tx.Model(&Conversation{}).Select("id").Where("project_id = ?", id)
		if err := tx.Where("conversation_id IN (?)", convIDs).Delete(&Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", id).Delete(&Conversation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", id).Delete(&KnowledgeFile{}).Error; err != nil {
			return err
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

// ---- Knowledge ----

// ListKnowledge returns a project's files without their content, oldest first.
func (s *Store) ListKnowledge(projectID string) ([]KnowledgeFile, error) {
	var files []KnowledgeFile
	err := s.db.Omit("content").Where("project_id = ?", projectID).Order("created_at ASC").Find(&files).Error
	return files, err
}

// ListKnowledgeWithContent returns a project's files including content,
// oldest first (the order they appear in the system prompt).
func (s *Store) ListKnowledgeWithContent(projectID string) ([]KnowledgeFile, error) {
	var files []KnowledgeFile
	err := s.db.Where("project_id = ?", projectID).Order("created_at ASC").Find(&files).Error
	return files, err
}

// CreateKnowledge inserts a knowledge file.
func (s *Store) CreateKnowledge(f *KnowledgeFile) error {
	f.ID = newID()
	return s.db.Create(f).Error
}

// GetKnowledge loads a knowledge file including its content.
func (s *Store) GetKnowledge(id string) (*KnowledgeFile, error) {
	var f KnowledgeFile
	if err := s.db.First(&f, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &f, nil
}

// SaveKnowledge writes all fields of f.
func (s *Store) SaveKnowledge(f *KnowledgeFile) error {
	return s.db.Save(f).Error
}

// DeleteKnowledge removes a knowledge file.
func (s *Store) DeleteKnowledge(id string) error {
	res := s.db.Where("id = ?", id).Delete(&KnowledgeFile{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Conversations ----

// ConversationFilter narrows ListConversations.
type ConversationFilter struct {
	// ProjectID limits to one project; "none" limits to chats outside any
	// project; empty means all.
	ProjectID string
	Query     string
	Starred   bool
	Limit     int
}

// ListConversations returns conversations, most recently updated first. The
// query matches titles and message contents.
func (s *Store) ListConversations(f ConversationFilter) ([]Conversation, error) {
	var convs []Conversation
	tx := s.db.Model(&Conversation{})
	switch f.ProjectID {
	case "":
	case "none":
		tx = tx.Where("project_id = ''")
	default:
		tx = tx.Where("project_id = ?", f.ProjectID)
	}
	if f.Starred {
		tx = tx.Where("starred = ?", true)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		p := likePattern(q)
		matching := s.db.Model(&Message{}).Select("conversation_id").Where(`content LIKE ? ESCAPE '\'`, p)
		tx = tx.Where(`title LIKE ? ESCAPE '\' OR id IN (?)`, p, matching)
	}
	if f.Limit > 0 {
		tx = tx.Limit(f.Limit)
	}
	err := tx.Order("updated_at DESC").Find(&convs).Error
	return convs, err
}

// CreateConversation inserts a conversation.
func (s *Store) CreateConversation(c *Conversation) error {
	c.ID = newID()
	return s.db.Create(c).Error
}

// GetConversation loads a conversation by ID.
func (s *Store) GetConversation(id string) (*Conversation, error) {
	var c Conversation
	if err := s.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// SaveConversation writes all fields of c.
func (s *Store) SaveConversation(c *Conversation) error {
	return s.db.Save(c).Error
}

// DeleteConversation removes a conversation and its messages.
func (s *Store) DeleteConversation(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", id).Delete(&Message{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&Conversation{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ---- Messages ----

// ListMessages returns a conversation's messages in order.
func (s *Store) ListMessages(conversationID string) ([]Message, error) {
	var msgs []Message
	err := s.db.Where("conversation_id = ?", conversationID).Order("created_at ASC, rowid ASC").Find(&msgs).Error
	return msgs, err
}

// CountMessages returns how many messages a conversation has.
func (s *Store) CountMessages(conversationID string) (int64, error) {
	var n int64
	err := s.db.Model(&Message{}).Where("conversation_id = ?", conversationID).Count(&n).Error
	return n, err
}

// CreateMessage inserts a message and bumps its conversation's UpdatedAt.
func (s *Store) CreateMessage(m *Message) error {
	m.ID = newID()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Model(&Conversation{}).Where("id = ?", m.ConversationID).Update("updated_at", time.Now()).Error
	})
}

// DeleteMessages removes the given messages.
func (s *Store) DeleteMessages(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.Where("id IN ?", ids).Delete(&Message{}).Error
}

// ---- Settings ----

// GetSetting decodes the JSON value stored under key into out. It reports
// false when the key has never been written.
func (s *Store) GetSetting(key string, out any) (bool, error) {
	var row Setting
	if err := s.db.First(&row, "key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal([]byte(row.Value), out)
}

// PutSetting stores v JSON-encoded under key.
func (s *Store) PutSetting(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.db.Save(&Setting{Key: key, Value: string(b), UpdatedAt: time.Now()}).Error
}

// ProjectNames maps every project ID to its name.
func (s *Store) ProjectNames() (map[string]string, error) {
	var rows []Project
	if err := s.db.Select("id", "name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, p := range rows {
		out[p.ID] = p.Name
	}
	return out, nil
}

// MemoryConversations returns a project's memory-enabled conversations
// updated after since (all of them when since is nil), newest first.
func (s *Store) MemoryConversations(projectID string, since *time.Time, limit int) ([]Conversation, error) {
	var convs []Conversation
	tx := s.db.Where("project_id = ? AND memory_disabled = ?", projectID, false)
	if since != nil {
		tx = tx.Where("updated_at > ?", *since)
	}
	err := tx.Order("updated_at DESC").Limit(limit).Find(&convs).Error
	return convs, err
}

// SetProjectMemory stores a project's memory without touching UpdatedAt,
// so a background memory refresh does not reorder the project list.
func (s *Store) SetProjectMemory(id, memory string, at time.Time) error {
	return s.db.Model(&Project{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"memory": memory, "memory_updated_at": at}).Error
}

// SetConversationTitle renames a conversation without bumping UpdatedAt.
func (s *Store) SetConversationTitle(id, title string) error {
	return s.db.Model(&Conversation{}).Where("id = ?", id).UpdateColumn("title", title).Error
}

// SetConversationModel records the model a conversation last used.
func (s *Store) SetConversationModel(id, model string) error {
	return s.db.Model(&Conversation{}).Where("id = ?", id).UpdateColumn("model", model).Error
}

// SaveConversationMeta writes a conversation's editable fields without
// bumping UpdatedAt, so renaming or starring keeps its place in "recent".
func (s *Store) SaveConversationMeta(c *Conversation) error {
	return s.db.Model(&Conversation{}).Where("id = ?", c.ID).UpdateColumns(map[string]any{
		"title":           c.Title,
		"starred":         c.Starred,
		"model":           c.Model,
		"project_id":      c.ProjectID,
		"memory_disabled": c.MemoryDisabled,
	}).Error
}
