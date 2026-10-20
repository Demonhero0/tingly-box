// Package channel serves the bindings between IM chats and projects: which
// project a chat talks to. Moving messages is the host's job (see
// .design/projects-im.md); this module only owns the table.
package channel

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// BindingsResponse lists channel bindings.
type BindingsResponse struct {
	Bindings []db.ChannelBinding `json:"bindings"`
}

// BindChannelRequest binds an IM chat to a project. A chat that is already
// bound moves to the project.
type BindChannelRequest struct {
	Platform  string `json:"platform" binding:"required"`
	ChatID    string `json:"chat_id" binding:"required"`
	ProjectID string `json:"project_id" binding:"required"`
	// ChatTitle is an optional display name for the chat.
	ChatTitle string `json:"chat_title,omitempty"`
}

// OKResponse acknowledges an action.
type OKResponse struct {
	OK bool `json:"ok"`
}

// Handler is the channel bindings HTTP module.
type Handler struct {
	svc *rt.Service
}

// NewHandler builds the module.
func NewHandler(svc *rt.Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts the binding endpoints.
func (h *Handler) RegisterRoutes(r *module.Routes) {
	api := r.API
	tag := swagger.WithTags("projects")
	api.GET("/projects/channel-bindings", h.list, tag,
		swagger.WithDescription("List the IM chats bound to projects, all of them or one project's"),
		swagger.WithQuery("project_id", "string", "Only this project's bindings"),
		swagger.WithResponseModel(BindingsResponse{}))
	api.PUT("/projects/channel-bindings", h.bind, tag,
		swagger.WithDescription("Bind an IM chat to a project; binding a bound chat again moves it"),
		swagger.WithRequestModel(BindChannelRequest{}),
		swagger.WithResponseModel(db.ChannelBinding{}))
	api.DELETE("/projects/channel-bindings/:id", h.unbind, tag,
		swagger.WithDescription("Unbind an IM chat"),
		swagger.WithResponseModel(OKResponse{}))
}

func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.Store.ListChannelBindings(c.Query("project_id"))
	if err != nil {
		apierr.Store(c, err, "bindings")
		return
	}
	c.JSON(http.StatusOK, BindingsResponse{Bindings: out})
}

func (h *Handler) bind(c *gin.Context) {
	var req BindChannelRequest
	if !apierr.Bind(c, &req) {
		return
	}
	b, err := h.svc.BindChannel(req.Platform, req.ChatID, req.ChatTitle, req.ProjectID)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, b)
	case errors.Is(err, db.ErrNotFound):
		apierr.Store(c, err, "project")
	case errors.Is(err, rt.ErrArchived):
		apierr.Message(c, http.StatusConflict, err.Error())
	case errors.Is(err, rt.ErrBindingInvalid):
		apierr.BadRequest(c, err.Error())
	default:
		apierr.Message(c, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) unbind(c *gin.Context) {
	if err := h.svc.Store.DeleteChannelBinding(c.Param("id")); err != nil {
		apierr.Store(c, err, "binding")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
