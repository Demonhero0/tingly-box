// Package project serves Projects: the project list and settings, the
// project conversation with its coordinator, the live event stream, memory,
// library and usage. Threads are served by the thread module.
package project

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/projects/llm"
	"github.com/tingly-dev/tingly-box/swagger"
)

// ThreadCounts is how many threads a project has in each Overview group.
type ThreadCounts struct {
	Working  int `json:"working"`
	Waiting  int `json:"waiting"`
	Ready    int `json:"ready"`
	Idle     int `json:"idle"`
	Resolved int `json:"resolved"`
}

// ProjectSummary is a project card.
type ProjectSummary struct {
	db.Project
	Threads ThreadCounts `json:"threads"`
}

// ProjectListResponse lists projects.
type ProjectListResponse struct {
	Projects []ProjectSummary `json:"projects"`
}

// ProjectDetailResponse is everything the project page needs at load.
type ProjectDetailResponse struct {
	Project db.Project   `json:"project"`
	Threads []db.Thread  `json:"threads"`
	Counts  ThreadCounts `json:"counts"`
	// CoordinatorBusy is true while the coordinator is working on a turn.
	CoordinatorBusy bool `json:"coordinator_busy"`
}

// CreateProjectRequest is the New project dialog.
type CreateProjectRequest struct {
	Name string `json:"name" binding:"required"`
	Goal string `json:"goal,omitempty"`
	// Repos are local repository paths; each thread branches from the
	// repository's current branch.
	Repos []string `json:"repos,omitempty"`
}

// UpdateProjectRequest changes the fields that are set.
type UpdateProjectRequest struct {
	Name                 *string   `json:"name,omitempty"`
	Goal                 *string   `json:"goal,omitempty"`
	Instructions         *string   `json:"instructions,omitempty"`
	Repos                *[]string `json:"repos,omitempty"`
	CoordinatorModel     *string   `json:"coordinator_model,omitempty"`
	ThreadModel          *string   `json:"thread_model,omitempty"`
	ThreadPermissionMode *string   `json:"thread_permission_mode,omitempty" enum:"default,acceptEdits,auto,bypassPermissions"`
	Starred              *bool     `json:"starred,omitempty"`
	Paused               *bool     `json:"paused,omitempty"`
	Archived             *bool     `json:"archived,omitempty"`
}

// MessagesResponse is the project conversation.
type MessagesResponse struct {
	Messages []db.ProjectMessage `json:"messages"`
}

// PostMessageRequest sends work or a question to the coordinator.
type PostMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

// StartSuggestionsRequest starts proposed threads; empty indexes start all.
type StartSuggestionsRequest struct {
	Indexes []int `json:"indexes,omitempty"`
}

// MemoryResponse lists memory files.
type MemoryResponse struct {
	Dir   string          `json:"dir"`
	Files []rt.MemoryFile `json:"files"`
}

// MemoryFileResponse is one memory file.
type MemoryFileResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// WriteMemoryRequest writes a memory file.
type WriteMemoryRequest struct {
	Content string `json:"content"`
}

// LibraryResponse lists library files.
type LibraryResponse struct {
	Dir   string           `json:"dir"`
	Files []rt.LibraryFile `json:"files"`
}

// UsageResponse is token use by model and by thread.
type UsageResponse struct {
	ByModel  []db.ModelUsage `json:"by_model"`
	ByThread []ThreadUsage   `json:"by_thread"`
}

// ThreadUsage is one thread's token use.
type ThreadUsage struct {
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	Turns        int    `json:"turns"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// InspectRepoRequest checks a local repository path.
type InspectRepoRequest struct {
	Path string `json:"path" binding:"required"`
}

// InspectRepoResponse is the concrete repository a path resolves to.
type InspectRepoResponse struct {
	OK     bool   `json:"ok"`
	Root   string `json:"root,omitempty"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ProjectModelsResponse lists the models the host offers. Error is set (and
// Models may be empty) when the list could not be fetched; a model ID can
// still be typed by hand.
type ProjectModelsResponse struct {
	Models []llm.Model `json:"models"`
	Error  string      `json:"error,omitempty"`
}

// ProjectRuntimeResponse is what threads run on.
type ProjectRuntimeResponse struct {
	// CLIPath is the Claude Code CLI; empty when none is installed.
	CLIPath string `json:"cli_path"`
	// BaseURL is the gateway threads send model calls to; empty when
	// Claude Code uses its own login.
	BaseURL string `json:"base_url"`
}

// OKResponse acknowledges an action.
type OKResponse struct {
	OK bool `json:"ok"`
}

// Handler is the Projects HTTP module.
type Handler struct {
	svc *rt.Service
}

// NewHandler builds the module.
func NewHandler(svc *rt.Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts the Projects endpoints.
func (h *Handler) RegisterRoutes(r *module.Routes) {
	api := r.API
	tag := swagger.WithTags("projects")
	api.GET("/projects", h.list, tag,
		swagger.WithDescription("List projects with thread counts per Overview group"),
		swagger.WithQuery("q", "string", "Search name and goal"),
		swagger.WithQuery("archived", "boolean", "List archived projects"),
		swagger.WithResponseModel(ProjectListResponse{}))
	api.POST("/projects", h.create, tag,
		swagger.WithDescription("Create a project"),
		swagger.WithRequestModel(CreateProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))
	api.GET("/projects/models", h.models, tag,
		swagger.WithDescription("Models the coordinator and threads can use"),
		swagger.WithResponseModel(ProjectModelsResponse{}))
	api.GET("/projects/runtime", h.runtime, tag,
		swagger.WithDescription("What threads run on: the Claude Code CLI and the gateway it is pointed at"),
		swagger.WithResponseModel(ProjectRuntimeResponse{}))
	api.POST("/projects/repos/inspect", h.inspectRepo, tag,
		swagger.WithDescription("Check a local repository path and return its root and current branch"),
		swagger.WithRequestModel(InspectRepoRequest{}),
		swagger.WithResponseModel(InspectRepoResponse{}))
	api.GET("/projects/:id", h.get, tag,
		swagger.WithDescription("Get a project with its threads"),
		swagger.WithResponseModel(ProjectDetailResponse{}))
	api.PATCH("/projects/:id", h.update, tag,
		swagger.WithDescription("Update project settings; pause, resume, archive, unarchive, star"),
		swagger.WithRequestModel(UpdateProjectRequest{}),
		swagger.WithResponseModel(db.Project{}))
	api.DELETE("/projects/:id", h.delete, tag,
		swagger.WithDescription("Delete a project with its threads, memory and library. Branches are kept."),
		swagger.WithResponseModel(OKResponse{}))

	api.GET("/projects/:id/messages", h.messages, tag,
		swagger.WithDescription("The project conversation"),
		swagger.WithQuery("limit", "integer", "Latest N messages"),
		swagger.WithResponseModel(MessagesResponse{}))
	api.POST("/projects/:id/messages", h.post, tag,
		swagger.WithDescription("Send work or a question to the coordinator; its reply arrives on the event stream"),
		swagger.WithRequestModel(PostMessageRequest{}),
		swagger.WithResponseModel(db.ProjectMessage{}))
	api.POST("/projects/:id/messages/:mid/start", h.startSuggestions, tag,
		swagger.WithDescription("Start threads the coordinator proposed"),
		swagger.WithRequestModel(StartSuggestionsRequest{}),
		swagger.WithResponseModel(db.ProjectMessage{}))
	api.GET("/projects/:id/events", h.events, tag,
		swagger.WithDescription("Server-sent events for the project page: message, message_delta, thread, thread_event, project"),
		swagger.WithResponseModel(rt.Event{}))

	api.GET("/projects/:id/memory", h.listMemory, tag,
		swagger.WithDescription("List memory files"),
		swagger.WithResponseModel(MemoryResponse{}))
	api.GET("/projects/:id/memory/:name", h.readMemory, tag,
		swagger.WithDescription("Read a memory file"),
		swagger.WithResponseModel(MemoryFileResponse{}))
	api.PUT("/projects/:id/memory/:name", h.writeMemory, tag,
		swagger.WithDescription("Write a memory file"),
		swagger.WithRequestModel(WriteMemoryRequest{}),
		swagger.WithResponseModel(MemoryFileResponse{}))
	api.DELETE("/projects/:id/memory/:name", h.deleteMemory, tag,
		swagger.WithDescription("Delete a memory file"),
		swagger.WithResponseModel(OKResponse{}))

	api.GET("/projects/:id/library", h.listLibrary, tag,
		swagger.WithDescription("List library files (uploads and thread outputs)"),
		swagger.WithResponseModel(LibraryResponse{}))
	api.POST("/projects/:id/library", h.uploadLibrary, tag,
		swagger.WithDescription("Upload files (multipart field \"files\") to the library"),
		swagger.WithResponseModel(LibraryResponse{}))
	api.GET("/projects/:id/library/file", h.downloadLibrary, tag,
		swagger.WithDescription("Download a library file"),
		swagger.WithQuery("path", "string", "Library-relative path"))
	api.DELETE("/projects/:id/library/file", h.deleteLibrary, tag,
		swagger.WithDescription("Delete a library file"),
		swagger.WithQuery("path", "string", "Library-relative path"),
		swagger.WithResponseModel(OKResponse{}))

	api.GET("/projects/:id/usage", h.usage, tag,
		swagger.WithDescription("Token use by model and by thread"),
		swagger.WithResponseModel(UsageResponse{}))
}

func counts(m map[string]int) ThreadCounts {
	return ThreadCounts{Working: m[db.ThreadWorking], Waiting: m[db.ThreadWaiting], Ready: m[db.ThreadReady], Idle: m[db.ThreadIdle], Resolved: m[db.ThreadResolved]}
}

// storeErr maps runtime errors to statuses.
func storeErr(c *gin.Context, err error, what string) {
	switch {
	case errors.Is(err, rt.ErrPaused), errors.Is(err, rt.ErrArchived), errors.Is(err, rt.ErrResolved):
		apierr.Message(c, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrNotFound):
		apierr.Store(c, err, what)
	default:
		apierr.Message(c, http.StatusBadRequest, err.Error())
	}
}

func (h *Handler) project(c *gin.Context) (*db.Project, bool) {
	p, err := h.svc.Store.GetProject(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "project")
		return nil, false
	}
	return p, true
}

func (h *Handler) list(c *gin.Context) {
	projects, err := h.svc.Store.ListProjects(c.Query("q"), c.Query("archived") == "true")
	if err != nil {
		apierr.Store(c, err, "projects")
		return
	}
	all, err := h.svc.Store.ThreadCounts()
	if err != nil {
		apierr.Store(c, err, "threads")
		return
	}
	out := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		out = append(out, ProjectSummary{Project: p, Threads: counts(all[p.ID])})
	}
	c.JSON(http.StatusOK, ProjectListResponse{Projects: out})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	p, err := h.svc.Create(c.Request.Context(), rt.NewProjectSpec{Name: req.Name, Goal: req.Goal, Repos: req.Repos})
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *Handler) models(c *gin.Context) {
	models, err := h.svc.Host.Models(c.Request.Context())
	resp := ProjectModelsResponse{Models: models}
	if resp.Models == nil {
		resp.Models = []llm.Model{}
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) runtime(c *gin.Context) {
	env, _ := h.svc.Host.ThreadEnv(c.Request.Context())
	resp := ProjectRuntimeResponse{CLIPath: rt.ClaudeCLIPath()}
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "ANTHROPIC_BASE_URL="); ok {
			resp.BaseURL = v
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) inspectRepo(c *gin.Context) {
	var req InspectRepoRequest
	if !apierr.Bind(c, &req) {
		return
	}
	repos, err := rt.ResolveRepos(c.Request.Context(), []string{req.Path})
	if err != nil {
		c.JSON(http.StatusOK, InspectRepoResponse{Error: err.Error()})
		return
	}
	info, err := rt.InspectRepo(c.Request.Context(), repos[0].Path)
	if err != nil {
		c.JSON(http.StatusOK, InspectRepoResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, InspectRepoResponse{OK: true, Root: info.Root, Branch: info.Branch, Dirty: info.Dirty})
}

func (h *Handler) get(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	threads, err := h.svc.Store.ListThreads(p.ID)
	if err != nil {
		apierr.Store(c, err, "threads")
		return
	}
	m := map[string]int{}
	for _, t := range threads {
		m[t.Status]++
	}
	if threads == nil {
		threads = []db.Thread{}
	}
	c.JSON(http.StatusOK, ProjectDetailResponse{Project: *p, Threads: threads, Counts: counts(m), CoordinatorBusy: h.svc.Coordinator.Busy(p.ID)})
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateProjectRequest
	if !apierr.Bind(c, &req) {
		return
	}
	p, ok := h.project(c)
	if !ok {
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
	if req.Goal != nil {
		p.Goal = strings.TrimSpace(*req.Goal)
	}
	if req.Instructions != nil {
		p.Instructions = *req.Instructions
	}
	if req.Repos != nil {
		repos, err := rt.ResolveRepos(c.Request.Context(), *req.Repos)
		if err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
		p.Repos = repos
	}
	if req.CoordinatorModel != nil {
		p.CoordinatorModel = strings.TrimSpace(*req.CoordinatorModel)
	}
	if req.ThreadModel != nil {
		p.ThreadModel = strings.TrimSpace(*req.ThreadModel)
	}
	if req.ThreadPermissionMode != nil {
		switch *req.ThreadPermissionMode {
		case "default", "acceptEdits", "auto", "bypassPermissions":
			p.ThreadPermissionMode = *req.ThreadPermissionMode
		default:
			apierr.BadRequest(c, "unknown permission mode")
			return
		}
	}
	if req.Starred != nil {
		p.Starred = *req.Starred
	}
	var err error
	switch {
	case req.Archived != nil && (*req.Archived != (p.ArchivedAt != nil)):
		err = h.svc.SetArchived(p, *req.Archived)
	case req.Paused != nil && *req.Paused != p.Paused:
		err = h.svc.SetPaused(p, *req.Paused)
	default:
		err = h.svc.Save(p)
	}
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) delete(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), p); err != nil {
		apierr.Store(c, err, "project")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) messages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	msgs, err := h.svc.Store.ListProjectMessages(c.Param("id"), limit)
	if err != nil {
		apierr.Store(c, err, "messages")
		return
	}
	if msgs == nil {
		msgs = []db.ProjectMessage{}
	}
	c.JSON(http.StatusOK, MessagesResponse{Messages: msgs})
}

func (h *Handler) post(c *gin.Context) {
	var req PostMessageRequest
	if !apierr.Bind(c, &req) {
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		apierr.BadRequest(c, "message is empty")
		return
	}
	m, err := h.svc.Coordinator.Post(c.Param("id"), req.Content)
	if err != nil {
		storeErr(c, err, "project")
		return
	}
	c.JSON(http.StatusCreated, m)
}

func (h *Handler) startSuggestions(c *gin.Context) {
	var req StartSuggestionsRequest
	if !apierr.Bind(c, &req) {
		return
	}
	m, err := h.svc.Coordinator.StartSuggestions(c.Request.Context(), c.Param("id"), c.Param("mid"), req.Indexes)
	if err != nil {
		storeErr(c, err, "message")
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) events(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	ch, cancel := h.svc.Hub.Subscribe(p.ID)
	defer cancel()
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	w.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case b := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			w.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			w.Flush()
		}
	}
}

func (h *Handler) listMemory(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	files, err := h.svc.Layout.ListMemory(p.ID)
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, MemoryResponse{Dir: h.svc.Layout.MemoryDir(p.ID), Files: files})
}

func (h *Handler) readMemory(c *gin.Context) {
	content, err := h.svc.Layout.ReadMemory(c.Param("id"), c.Param("name"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			apierr.Message(c, http.StatusNotFound, "memory file not found")
			return
		}
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, MemoryFileResponse{Name: c.Param("name"), Content: content})
}

func (h *Handler) writeMemory(c *gin.Context) {
	var req WriteMemoryRequest
	if !apierr.Bind(c, &req) {
		return
	}
	p, ok := h.project(c)
	if !ok {
		return
	}
	if err := h.svc.Layout.WriteMemory(p.ID, c.Param("name"), req.Content); err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, MemoryFileResponse{Name: c.Param("name"), Content: req.Content})
}

func (h *Handler) deleteMemory(c *gin.Context) {
	if err := h.svc.Layout.DeleteMemory(c.Param("id"), c.Param("name")); err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) listLibrary(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	files, err := h.svc.Layout.ListLibrary(p.ID)
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, LibraryResponse{Dir: h.svc.Layout.LibraryDir(p.ID), Files: files})
}

// maxUpload caps one uploaded library file.
const maxUpload = 200 << 20

func (h *Handler) uploadLibrary(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		apierr.BadRequest(c, `expected multipart form with field "files"`)
		return
	}
	for _, fh := range form.File["files"] {
		if fh.Size > maxUpload {
			apierr.BadRequest(c, fmt.Sprintf("%s is larger than 200 MB", fh.Filename))
			return
		}
		dst, err := h.svc.Layout.LibraryPath(p.ID, filepath.Base(fh.Filename))
		if err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
		if err := saveUpload(func() (multipartFile, error) { return fh.Open() }, dst); err != nil {
			apierr.Message(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.listLibrary(c)
}

func saveUpload(open func() (multipartFile, error), dst string) error {
	src, err := open()
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

func (h *Handler) downloadLibrary(c *gin.Context) {
	path, err := h.svc.Layout.LibraryPath(c.Param("id"), c.Query("path"))
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		apierr.Message(c, http.StatusNotFound, "file not found")
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}

func (h *Handler) deleteLibrary(c *gin.Context) {
	path, err := h.svc.Layout.LibraryPath(c.Param("id"), c.Query("path"))
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	if err := os.Remove(path); err != nil {
		apierr.Message(c, http.StatusNotFound, "file not found")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) usage(c *gin.Context) {
	p, ok := h.project(c)
	if !ok {
		return
	}
	byModel, err := h.svc.Store.ProjectUsage(p.ID)
	if err != nil {
		apierr.Store(c, err, "usage")
		return
	}
	threads, _ := h.svc.Store.ListThreads(p.ID)
	resp := UsageResponse{ByModel: byModel, ByThread: []ThreadUsage{}}
	if resp.ByModel == nil {
		resp.ByModel = []db.ModelUsage{}
	}
	for _, t := range threads {
		resp.ByThread = append(resp.ByThread, ThreadUsage{ThreadID: t.ID, Title: t.Title, Model: t.Model, Turns: t.Turns, InputTokens: t.InputTokens, OutputTokens: t.OutputTokens})
	}
	c.JSON(http.StatusOK, resp)
}

// multipartFile is what a multipart file header opens.
type multipartFile = io.ReadCloser
