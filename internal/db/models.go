// Package db owns persistence for tingly-project: the gorm models and the
// Store that every HTTP module reads and writes through.
package db

import (
	"time"
)

// Project groups conversations around shared instructions and knowledge,
// the same unit as a Claude Project.
type Project struct {
	ID           string `gorm:"primaryKey;size:36" json:"id"`
	Name         string `gorm:"size:200;not null" json:"name"`
	Description  string `gorm:"type:text" json:"description"`
	Instructions string `gorm:"type:text" json:"instructions"`
	// Memory is a model-written summary of what was learned across the
	// project's chats. It is part of every chat's context and user-editable.
	Memory          string     `gorm:"type:text" json:"memory"`
	MemoryUpdatedAt *time.Time `json:"memory_updated_at,omitempty"`
	Starred         bool       `json:"starred"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// KnowledgeFile is a text document attached to a project. Its content is
// placed in the system prompt of every conversation in that project.
type KnowledgeFile struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	ProjectID string    `gorm:"size:36;index;not null" json:"project_id"`
	Name      string    `gorm:"size:300;not null" json:"name"`
	MimeType  string    `gorm:"size:100" json:"mime_type"`
	Content   string    `gorm:"type:text" json:"content"`
	Size      int64     `json:"size"`
	Tokens    int       `json:"tokens"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Conversation is one chat thread. ProjectID is empty for chats that do not
// belong to a project.
type Conversation struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	ProjectID string `gorm:"size:36;index" json:"project_id"`
	Title     string `gorm:"size:300" json:"title"`
	Model     string `gorm:"size:200" json:"model"`
	Starred   bool   `json:"starred"`
	// MemoryDisabled chats neither read nor feed project memory. It can only
	// be chosen before the first message, like claude.ai.
	MemoryDisabled bool      `json:"memory_disabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one turn in a conversation.
type Message struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	ConversationID string    `gorm:"size:36;index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:20;not null" json:"role"`
	Content        string    `gorm:"type:text" json:"content"`
	Model          string    `gorm:"size:200" json:"model,omitempty"`
	InputTokens    int       `json:"input_tokens,omitempty"`
	OutputTokens   int       `json:"output_tokens,omitempty"`
	StopReason     string    `gorm:"size:50" json:"stop_reason,omitempty"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Setting is a key/value row holding a JSON-encoded settings section.
type Setting struct {
	Key       string `gorm:"primaryKey;size:100"`
	Value     string `gorm:"type:text"`
	UpdatedAt time.Time
}
