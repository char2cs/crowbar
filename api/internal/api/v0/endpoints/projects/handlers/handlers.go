package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds project HTTP handlers.
type Handlers struct {
	svc usecases.ProjectUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.ProjectUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// Create handles POST /projects.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "bad request", "")
		return
	}
	p, err := h.svc.Create(c.Request.Context(), body.Name)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), "")
		return
	}
	c.JSON(http.StatusCreated, libs.DataResponse(p))
}

// List handles GET /projects.
func (h *Handlers) List(
	c *gin.Context,
) {
	projects, err := h.svc.List(c.Request.Context())
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), "")
		return
	}
	libs.WriteQueryOK(c, projects)
}

// Get handles GET /projects/:id.
func (h *Handlers) Get(
	c *gin.Context,
) {
	id := c.Param("id")
	p, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, p)
}

// Delete handles DELETE /projects/:id.
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
