// Package conversation serves chats: listing, moving between projects, and
// the streaming chat turn that assembles project context and calls the model.
package conversation

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/swagger"
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/server/module"
	"github.com/tingly-dev/tingly-project/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-project/internal/server/module/settings"
)

// ConversationSummary is a conversation as listed in the sidebar and lists.
type ConversationSummary struct {
	db.Conversation
	// ProjectName is empty for chats outside any project.
	ProjectName string `json:"project_name"`
}

// ConversationListResponse lists conversations.
type ConversationListResponse struct {
	Conversations []ConversationSummary `json:"conversations"`
}

// ProjectRef identifies the project a conversation belongs to.
type ProjectRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

// ConversationDetailResponse is a conversation with its messages.
type ConversationDetailResponse struct {
	Conversation db.Conversation `json:"conversation"`
	Project      *ProjectRef     `json:"project,omitempty"`
	Messages     []db.Message    `json:"messages"`
}

// CreateConversationRequest starts a conversation.
type CreateConversationRequest struct {
	ProjectID      string `json:"project_id,omitempty"`
	Model          string `json:"model,omitempty"`
	MemoryDisabled bool   `json:"memory_disabled,omitempty"`
}

// UpdateConversationRequest changes the fields that are set. ProjectID ""
// removes the chat from its project.
type UpdateConversationRequest struct {
	Title          *string `json:"title,omitempty"`
	Starred        *bool   `json:"starred,omitempty"`
	Model          *string `json:"model,omitempty"`
	ProjectID      *string `json:"project_id,omitempty"`
	MemoryDisabled *bool   `json:"memory_disabled,omitempty"`
}

// OKResponse acknowledges a deletion.
type OKResponse struct {
	OK bool `json:"ok"`
}

// MemoryScheduler queues a project memory refresh.
type MemoryScheduler interface {
	Schedule(projectID string)
}

// Handler is the conversation HTTP module.
type Handler struct {
	store    *db.Store
	settings *settings.Service
	memory   MemoryScheduler

	busyMu sync.Mutex
	busy   map[string]bool
}

// NewHandler builds the conversation module.
func NewHandler(store *db.Store, settings *settings.Service, memory MemoryScheduler) *Handler {
	return &Handler{store: store, settings: settings, memory: memory, busy: map[string]bool{}}
}

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts conversation endpoints.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	api := rt.API
	api.GET("/conversations", h.list,
		swagger.WithTags("conversations"),
		swagger.WithDescription("List conversations, most recent first"),
		swagger.WithQuery("project_id", "string", `Project ID, or "none" for chats outside any project`),
		swagger.WithQuery("q", "string", "Search titles and message text"),
		swagger.WithQuery("starred", "boolean", "Only starred conversations"),
		swagger.WithQuery("limit", "integer", "Maximum number of conversations"),
		swagger.WithResponseModel(ConversationListResponse{}))
	api.POST("/conversations", h.create,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Start a conversation, optionally inside a project"),
		swagger.WithRequestModel(CreateConversationRequest{}),
		swagger.WithResponseModel(db.Conversation{}))
	api.GET("/conversations/:id", h.get,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Get a conversation with its messages"),
		swagger.WithResponseModel(ConversationDetailResponse{}))
	api.PATCH("/conversations/:id", h.update,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Rename, star, change model, or move a conversation between projects"),
		swagger.WithRequestModel(UpdateConversationRequest{}),
		swagger.WithResponseModel(db.Conversation{}))
	api.DELETE("/conversations/:id", h.delete,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Delete a conversation"),
		swagger.WithResponseModel(OKResponse{}))
	api.POST("/conversations/:id/messages", h.send,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Send a user message and stream the reply as text/event-stream; each data line is a StreamEvent"),
		swagger.WithRequestModel(SendMessageRequest{}),
		swagger.WithResponseModel(StreamEvent{}))
	api.POST("/conversations/:id/regenerate", h.regenerate,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Replace the last reply and stream a new one as text/event-stream"),
		swagger.WithRequestModel(RegenerateRequest{}),
		swagger.WithResponseModel(StreamEvent{}))
}

func (h *Handler) list(c *gin.Context) {
	limit := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	convs, err := h.store.ListConversations(db.ConversationFilter{
		ProjectID: c.Query("project_id"),
		Query:     c.Query("q"),
		Limit:     limit,
		Starred:   c.Query("starred") == "true",
	})
	if err != nil {
		apierr.Store(c, err, "conversations")
		return
	}
	names, err := h.store.ProjectNames()
	if err != nil {
		apierr.Store(c, err, "projects")
		return
	}
	out := make([]ConversationSummary, 0, len(convs))
	for _, cv := range convs {
		out = append(out, ConversationSummary{Conversation: cv, ProjectName: names[cv.ProjectID]})
	}
	c.JSON(http.StatusOK, ConversationListResponse{Conversations: out})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateConversationRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if req.ProjectID != "" {
		if _, err := h.store.GetProject(req.ProjectID); err != nil {
			apierr.Store(c, err, "project")
			return
		}
	}
	conv := &db.Conversation{
		ProjectID:      req.ProjectID,
		Model:          strings.TrimSpace(req.Model),
		MemoryDisabled: req.MemoryDisabled,
	}
	if err := h.store.CreateConversation(conv); err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	c.JSON(http.StatusCreated, conv)
}

func (h *Handler) get(c *gin.Context) {
	conv, err := h.store.GetConversation(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	msgs, err := h.store.ListMessages(conv.ID)
	if err != nil {
		apierr.Store(c, err, "messages")
		return
	}
	resp := ConversationDetailResponse{Conversation: *conv, Messages: msgs}
	if resp.Messages == nil {
		resp.Messages = []db.Message{}
	}
	if conv.ProjectID != "" {
		if p, err := h.store.GetProject(conv.ProjectID); err == nil {
			resp.Project = &ProjectRef{ID: p.ID, Name: p.Name, Archived: p.ArchivedAt != nil}
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateConversationRequest
	if !apierr.Bind(c, &req) {
		return
	}
	conv, err := h.store.GetConversation(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	if req.Title != nil {
		conv.Title = strings.TrimSpace(*req.Title)
	}
	if req.Starred != nil {
		conv.Starred = *req.Starred
	}
	if req.Model != nil {
		conv.Model = strings.TrimSpace(*req.Model)
	}
	if req.MemoryDisabled != nil && *req.MemoryDisabled != conv.MemoryDisabled {
		n, err := h.store.CountMessages(conv.ID)
		if err != nil {
			apierr.Store(c, err, "messages")
			return
		}
		if n > 0 {
			apierr.Message(c, http.StatusConflict, "memory can only be turned off before the first message")
			return
		}
		conv.MemoryDisabled = *req.MemoryDisabled
	}
	if req.ProjectID != nil && *req.ProjectID != conv.ProjectID {
		if *req.ProjectID != "" {
			p, err := h.store.GetProject(*req.ProjectID)
			if err != nil {
				apierr.Store(c, err, "project")
				return
			}
			if p.ArchivedAt != nil {
				apierr.Message(c, http.StatusConflict, "cannot move a chat into an archived project")
				return
			}
		}
		conv.ProjectID = *req.ProjectID
	}
	// Edits are metadata: keep the conversation's place in "recent" order.
	if err := h.store.SaveConversationMeta(conv); err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	c.JSON(http.StatusOK, conv)
}

func (h *Handler) delete(c *gin.Context) {
	if err := h.store.DeleteConversation(c.Param("id")); err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// acquire marks a conversation busy for one streaming turn. It reports false
// if a turn is already running.
func (h *Handler) acquire(id string) bool {
	h.busyMu.Lock()
	defer h.busyMu.Unlock()
	if h.busy[id] {
		return false
	}
	h.busy[id] = true
	return true
}

func (h *Handler) release(id string) {
	h.busyMu.Lock()
	delete(h.busy, id)
	h.busyMu.Unlock()
}
