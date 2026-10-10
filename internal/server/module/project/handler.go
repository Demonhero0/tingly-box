// Package project serves projects and their knowledge: the custom
// instructions and documents that every conversation in a project starts with.
package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/swagger"
	"github.com/tingly-dev/tingly-project/internal/db"
	"github.com/tingly-dev/tingly-project/internal/knowledge"
	"github.com/tingly-dev/tingly-project/internal/server/module"
	"github.com/tingly-dev/tingly-project/internal/server/module/apierr"
)

// ProjectSummary is a project as listed on the projects page.
type ProjectSummary struct {
	db.Project
	ConversationCount int       `json:"conversation_count"`
	KnowledgeCount    int       `json:"knowledge_count"`
	KnowledgeTokens   int       `json:"knowledge_tokens"`
	LastActivityAt    time.Time `json:"last_activity_at"`
}

// ProjectListResponse lists projects.
type ProjectListResponse struct {
	Projects []ProjectSummary `json:"projects"`
}

// KnowledgeFileSummary is a knowledge file without its content.
type KnowledgeFileSummary struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	Tokens    int       `json:"tokens"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Capacity is how much knowledge the project holds against its budgets.
type Capacity struct {
	// KnowledgeTokens is the estimated size of all knowledge files.
	KnowledgeTokens int `json:"knowledge_tokens"`
	// InlineBudget is the size up to which knowledge is sent in full.
	InlineBudget int `json:"inline_budget"`
	// MaxTokens is the hard cap, reachable in retrieval mode.
	MaxTokens int `json:"max_tokens"`
	// Mode is "inline" or "retrieval"; it switches automatically.
	Mode string `json:"mode" enum:"inline,retrieval"`
}

// ProjectDetailResponse is a project with its knowledge.
type ProjectDetailResponse struct {
	Project   db.Project             `json:"project"`
	Knowledge []KnowledgeFileSummary `json:"knowledge"`
	Capacity  Capacity               `json:"capacity"`
}

// CreateProjectRequest creates a project.
type CreateProjectRequest struct {
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// UpdateProjectRequest changes the fields that are set.
type UpdateProjectRequest struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	Instructions *string `json:"instructions,omitempty"`
	Memory       *string `json:"memory,omitempty"`
	Starred      *bool   `json:"starred,omitempty"`
	Archived     *bool   `json:"archived,omitempty"`
}

// ContextResponse is the exact system prompt a project's conversations get.
// In retrieval mode it is computed for the query parameter "q".
type ContextResponse struct {
	SystemPrompt    string `json:"system_prompt"`
	Tokens          int    `json:"tokens"`
	Mode            string `json:"mode"`
	KnowledgeTokens int    `json:"knowledge_tokens"`
	Excerpts        int    `json:"excerpts"`
}

// MemoryRefresher folds recent chats into a project's memory.
type MemoryRefresher interface {
	Refresh(ctx context.Context, projectID string) (*db.Project, error)
}

// AddTextRequest adds pasted text as a knowledge file.
type AddTextRequest struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// UpdateKnowledgeRequest renames or rewrites a knowledge file.
type UpdateKnowledgeRequest struct {
	Name    *string `json:"name,omitempty"`
	Content *string `json:"content,omitempty"`
}

// RejectedFile is an upload that could not be used as knowledge.
type RejectedFile struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// UploadResponse reports accepted and rejected uploads.
type UploadResponse struct {
	Files    []KnowledgeFileSummary `json:"files"`
	Rejected []RejectedFile         `json:"rejected"`
}

// OKResponse acknowledges a deletion.
type OKResponse struct {
	OK bool `json:"ok"`
}

// Handler is the project HTTP module.
type Handler struct {
	store  *db.Store
	memory MemoryRefresher
}

// NewHandler builds the project module.
func NewHandler(store *db.Store, memory MemoryRefresher) *Handler {
	return &Handler{store: store, memory: memory}
}

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts project and knowledge endpoints.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	api := rt.API
	api.GET("/projects", h.list,
		swagger.WithTags("projects"),
		swagger.WithDescription("List projects"),
		swagger.WithQuery("q", "string", "Search name and description"),
		swagger.WithQuery("archived", "boolean", "List archived projects instead of active ones"),
		swagger.WithResponseModel(ProjectListResponse{}))
	api.POST("/projects", h.create,
		swagger.WithTags("projects"),
		swagger.WithDescription("Create a project"),
		swagger.WithRequestModel(CreateProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))
	api.GET("/projects/:id", h.get,
		swagger.WithTags("projects"),
		swagger.WithDescription("Get a project with its knowledge"),
		swagger.WithResponseModel(ProjectDetailResponse{}))
	api.PATCH("/projects/:id", h.update,
		swagger.WithTags("projects"),
		swagger.WithDescription("Update a project"),
		swagger.WithRequestModel(UpdateProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))
	api.DELETE("/projects/:id", h.delete,
		swagger.WithTags("projects"),
		swagger.WithDescription("Delete a project with its knowledge and conversations. Archived projects must be unarchived first."),
		swagger.WithResponseModel(OKResponse{}))
	api.GET("/projects/:id/context", h.context,
		swagger.WithTags("projects"),
		swagger.WithDescription("Get the system prompt sent with conversations in the project"),
		swagger.WithQuery("q", "string", "Sample user message to rank knowledge against in retrieval mode"),
		swagger.WithResponseModel(ContextResponse{}))
	api.POST("/projects/:id/memory/refresh", h.refreshMemory,
		swagger.WithTags("projects"),
		swagger.WithDescription("Fold the project's recent chats into its memory now"),
		swagger.WithResponseModel(db.Project{}))

	api.POST("/projects/:id/knowledge", h.upload,
		swagger.WithTags("knowledge"),
		swagger.WithDescription("Upload text files (multipart field \"files\") as project knowledge"),
		swagger.WithResponseModel(UploadResponse{}))
	api.POST("/projects/:id/knowledge/text", h.addText,
		swagger.WithTags("knowledge"),
		swagger.WithDescription("Add pasted text as project knowledge"),
		swagger.WithRequestModel(AddTextRequest{}),
		swagger.WithResponseModel(KnowledgeFileSummary{}))
	api.GET("/knowledge/:id", h.getKnowledge,
		swagger.WithTags("knowledge"),
		swagger.WithDescription("Get a knowledge file with its content"),
		swagger.WithResponseModel(db.KnowledgeFile{}))
	api.PATCH("/knowledge/:id", h.updateKnowledge,
		swagger.WithTags("knowledge"),
		swagger.WithDescription("Rename or rewrite a knowledge file"),
		swagger.WithRequestModel(UpdateKnowledgeRequest{}),
		swagger.WithResponseModel(KnowledgeFileSummary{}))
	api.DELETE("/knowledge/:id", h.deleteKnowledge,
		swagger.WithTags("knowledge"),
		swagger.WithDescription("Delete a knowledge file"),
		swagger.WithResponseModel(OKResponse{}))
}

func summarize(f db.KnowledgeFile) KnowledgeFileSummary {
	return KnowledgeFileSummary{
		ID: f.ID, ProjectID: f.ProjectID, Name: f.Name, MimeType: f.MimeType,
		Size: f.Size, Tokens: f.Tokens, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

func (h *Handler) list(c *gin.Context) {
	projects, stats, err := h.store.ListProjects(c.Query("q"), c.Query("archived") == "true")
	if err != nil {
		apierr.Store(c, err, "projects")
		return
	}
	out := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		st := stats[p.ID]
		out = append(out, ProjectSummary{
			Project:           p,
			ConversationCount: st.ConversationCount,
			KnowledgeCount:    st.KnowledgeCount,
			KnowledgeTokens:   st.KnowledgeTokens,
			LastActivityAt:    st.LastActivityAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Starred != out[j].Starred {
			return out[i].Starred
		}
		return out[i].LastActivityAt.After(out[j].LastActivityAt)
	})
	c.JSON(http.StatusOK, ProjectListResponse{Projects: out})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		apierr.BadRequest(c, "name is required")
		return
	}
	p := &db.Project{Name: name, Description: strings.TrimSpace(req.Description), Instructions: req.Instructions}
	if err := h.store.CreateProject(p); err != nil {
		apierr.Store(c, err, "project")
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *Handler) get(c *gin.Context) {
	p, err := h.store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}
	files, err := h.store.ListKnowledge(p.ID)
	if err != nil {
		apierr.Store(c, err, "knowledge")
		return
	}
	resp := ProjectDetailResponse{
		Project:   *p,
		Knowledge: make([]KnowledgeFileSummary, 0, len(files)),
		Capacity: Capacity{
			InlineBudget: knowledge.InlineBudget,
			MaxTokens:    knowledge.MaxKnowledgeTokens,
		},
	}
	for _, f := range files {
		resp.Knowledge = append(resp.Knowledge, summarize(f))
		resp.Capacity.KnowledgeTokens += f.Tokens
	}
	resp.Capacity.Mode = knowledge.ModeFor(resp.Capacity.KnowledgeTokens)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	p, err := h.store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			apierr.BadRequest(c, "name cannot be empty")
			return
		}
		p.Name = name
	}
	if req.Description != nil {
		p.Description = strings.TrimSpace(*req.Description)
	}
	if req.Instructions != nil {
		p.Instructions = *req.Instructions
	}
	if req.Memory != nil {
		p.Memory = strings.TrimSpace(*req.Memory)
		now := time.Now()
		p.MemoryUpdatedAt = &now
	}
	if req.Starred != nil {
		p.Starred = *req.Starred
	}
	if req.Archived != nil {
		if *req.Archived && p.ArchivedAt == nil {
			now := time.Now()
			p.ArchivedAt = &now
		} else if !*req.Archived {
			p.ArchivedAt = nil
		}
	}
	if err := h.store.SaveProject(p); err != nil {
		apierr.Store(c, err, "project")
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) delete(c *gin.Context) {
	p, err := h.store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}
	if p.ArchivedAt != nil {
		apierr.Message(c, http.StatusConflict, "unarchive the project before deleting it")
		return
	}
	if err := h.store.DeleteProject(p.ID); err != nil {
		apierr.Store(c, err, "project")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) context(c *gin.Context) {
	p, err := h.store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}
	files, err := h.store.ListKnowledgeWithContent(p.ID)
	if err != nil {
		apierr.Store(c, err, "knowledge")
		return
	}
	prompt := knowledge.Build(knowledge.PromptInput{Project: p, Files: files, Query: c.Query("q"), Memory: true})
	c.JSON(http.StatusOK, ContextResponse{
		SystemPrompt:    prompt.System,
		Tokens:          knowledge.EstimateTokens(prompt.System),
		Mode:            prompt.Mode,
		KnowledgeTokens: prompt.KnowledgeTokens,
		Excerpts:        prompt.Excerpts,
	})
}

func (h *Handler) refreshMemory(c *gin.Context) {
	p, err := h.memory.Refresh(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			apierr.Store(c, err, "project")
			return
		}
		apierr.Message(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, p)
}

// knowledgeTokens is the project's current knowledge size.
func (h *Handler) knowledgeTokens(projectID string) (int, error) {
	files, err := h.store.ListKnowledge(projectID)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, f := range files {
		total += f.Tokens
	}
	return total, nil
}

func overCapacity() string {
	return fmt.Sprintf("project knowledge would exceed %dk tokens", knowledge.MaxKnowledgeTokens/1000)
}

func (h *Handler) upload(c *gin.Context) {
	p, err := h.store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		apierr.BadRequest(c, "expected multipart form with field \"files\"")
		return
	}
	total, err := h.knowledgeTokens(p.ID)
	if err != nil {
		apierr.Store(c, err, "knowledge")
		return
	}
	resp := UploadResponse{Files: []KnowledgeFileSummary{}, Rejected: []RejectedFile{}}
	for _, fh := range form.File["files"] {
		reject := func(reason string) {
			resp.Rejected = append(resp.Rejected, RejectedFile{Name: fh.Filename, Reason: reason})
		}
		if fh.Size > knowledge.MaxFileBytes {
			reject("file is larger than 10 MB")
			continue
		}
		f, err := fh.Open()
		if err != nil {
			reject(err.Error())
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, knowledge.MaxFileBytes+1))
		_ = f.Close()
		if err != nil {
			reject(err.Error())
			continue
		}
		content, mimeType, err := knowledge.Decode(fh.Filename, data)
		if err != nil {
			reject(err.Error())
			continue
		}
		tokens := knowledge.EstimateTokens(content)
		if total+tokens > knowledge.MaxKnowledgeTokens {
			reject(overCapacity())
			continue
		}
		kf := &db.KnowledgeFile{
			ProjectID: p.ID,
			Name:      fh.Filename,
			MimeType:  mimeType,
			Content:   content,
			Size:      int64(len(data)),
			Tokens:    tokens,
		}
		if err := h.store.CreateKnowledge(kf); err != nil {
			reject(err.Error())
			continue
		}
		total += tokens
		resp.Files = append(resp.Files, summarize(*kf))
	}
	if len(resp.Files) > 0 {
		_ = h.store.TouchProject(p.ID)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) addText(c *gin.Context) {
	var req AddTextRequest
	if !apierr.Bind(c, &req) {
		return
	}
	p, err := h.store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		apierr.BadRequest(c, "name is required")
		return
	}
	total, err := h.knowledgeTokens(p.ID)
	if err != nil {
		apierr.Store(c, err, "knowledge")
		return
	}
	if tokens := knowledge.EstimateTokens(req.Content); total+tokens > knowledge.MaxKnowledgeTokens {
		apierr.BadRequest(c, overCapacity())
		return
	}
	kf := &db.KnowledgeFile{
		ProjectID: p.ID,
		Name:      name,
		MimeType:  "text/plain",
		Content:   req.Content,
		Size:      int64(len(req.Content)),
		Tokens:    knowledge.EstimateTokens(req.Content),
	}
	if err := h.store.CreateKnowledge(kf); err != nil {
		apierr.Store(c, err, "knowledge")
		return
	}
	_ = h.store.TouchProject(p.ID)
	c.JSON(http.StatusCreated, summarize(*kf))
}

func (h *Handler) getKnowledge(c *gin.Context) {
	f, err := h.store.GetKnowledge(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "knowledge file")
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *Handler) updateKnowledge(c *gin.Context) {
	var req UpdateKnowledgeRequest
	if !apierr.Bind(c, &req) {
		return
	}
	f, err := h.store.GetKnowledge(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "knowledge file")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			apierr.BadRequest(c, "name cannot be empty")
			return
		}
		f.Name = name
	}
	if req.Content != nil {
		f.Content = *req.Content
		f.Size = int64(len(f.Content))
		f.Tokens = knowledge.EstimateTokens(f.Content)
	}
	if err := h.store.SaveKnowledge(f); err != nil {
		apierr.Store(c, err, "knowledge file")
		return
	}
	c.JSON(http.StatusOK, summarize(*f))
}

func (h *Handler) deleteKnowledge(c *gin.Context) {
	if err := h.store.DeleteKnowledge(c.Param("id")); err != nil {
		apierr.Store(c, err, "knowledge file")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
