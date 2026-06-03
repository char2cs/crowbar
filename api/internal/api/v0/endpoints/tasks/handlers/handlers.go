package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds task HTTP handlers.
type Handlers struct {
	svc usecases.TaskUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.TaskUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// Create handles POST /repositories/:id/tasks.
func (h *Handlers) Create(
	c *gin.Context,
) {
	repositoryID := c.Param("id")
	var body struct {
		Title      string `json:"title" binding:"required"`
		BranchName string `json:"branch_name" binding:"required"`
		BaseBranch string `json:"base_branch" binding:"required"`
		FlowPath   string `json:"flow_path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "bad request", "")
		return
	}
	task, err := h.svc.Create(
		c.Request.Context(),
		repositoryID,
		body.Title,
		body.BranchName,
		body.BaseBranch,
		body.FlowPath,
	)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", repositoryID)
			return
		}
		if errors.Is(err, repositories.ErrBadRequest) {
			libs.WriteErr(c, http.StatusBadRequest, err.Error(), "")
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), "")
		return
	}
	c.JSON(http.StatusCreated, libs.DataResponse(task))
}

// List handles GET /repositories/:id/tasks.
func (h *Handlers) List(
	c *gin.Context,
) {
	repositoryID := c.Param("id")
	tasks, err := h.svc.List(c.Request.Context(), repositoryID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", repositoryID)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), "")
		return
	}
	libs.WriteQueryOK(c, tasks)
}

// Get handles GET /tasks/:id.
func (h *Handlers) Get(
	c *gin.Context,
) {
	id := c.Param("id")
	task, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, task)
}

// Archive handles POST /tasks/:id/archive.
func (h *Handlers) Archive(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.svc.Archive(c.Request.Context(), id); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	task, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, task)
}

// Pause handles POST /tasks/:id/pause.
func (h *Handlers) Pause(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.svc.Pause(c.Request.Context(), id); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	task, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, task)
}

// Resume handles POST /tasks/:id/resume.
func (h *Handlers) Resume(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.svc.Resume(c.Request.Context(), id); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	task, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, task)
}

// Retry handles POST /tasks/:id/retry.
func (h *Handlers) Retry(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.svc.Retry(c.Request.Context(), id); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	task, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, task)
}

// ForceTransition handles POST /tasks/:id/force-transition.
func (h *Handlers) ForceTransition(
	c *gin.Context,
) {
	id := c.Param("id")
	var body struct {
		ToState string `json:"to_state" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "bad request", id)
		return
	}
	if err := h.svc.ForceTransition(c.Request.Context(), id, body.ToState); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		if errors.Is(err, repositories.ErrBadRequest) {
			libs.WriteErr(c, http.StatusBadRequest, err.Error(), id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	updated, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, updated)
}
