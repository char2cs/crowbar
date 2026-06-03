package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds repository HTTP handlers.
type Handlers struct {
	svc usecases.RepositoryUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.RepositoryUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// Create handles POST /projects/:id/repositories.
func (h *Handlers) Create(
	c *gin.Context,
) {
	projectID := c.Param("id")
	var body struct {
		Name string `json:"name" binding:"required"`
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "bad request", projectID)
		return
	}
	repo, err := h.svc.Create(c.Request.Context(), projectID, body.Name, body.Path)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", projectID)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), projectID)
		return
	}
	c.JSON(http.StatusCreated, libs.DataResponse(repo))
}

// List handles GET /projects/:id/repositories.
func (h *Handlers) List(
	c *gin.Context,
) {
	projectID := c.Param("id")
	repos, err := h.svc.List(c.Request.Context(), projectID)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), projectID)
		return
	}
	libs.WriteQueryOK(c, repos)
}

// Get handles GET /repositories/:id.
func (h *Handlers) Get(
	c *gin.Context,
) {
	id := c.Param("id")
	repo, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, repo)
}

// Delete handles DELETE /repositories/:id.
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	c.Status(http.StatusNoContent)
}
