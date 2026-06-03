package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds review-thread HTTP handlers.
type Handlers struct {
	svc usecases.ReviewThreadUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.ReviewThreadUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// List handles GET /tasks/:id/threads.
func (h *Handlers) List(
	c *gin.Context,
) {
	taskID := c.Param("id")
	status := c.Query("status")
	phase := c.Query("phase")
	threads, err := h.svc.List(c.Request.Context(), taskID, status, phase)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), taskID)
		return
	}
	libs.WriteQueryOK(c, threads)
}

// Open handles POST /tasks/:id/threads.
func (h *Handlers) Open(
	c *gin.Context,
) {
	taskID := c.Param("id")
	var body struct {
		File    string `json:"file_path" binding:"required"`
		Line    int    `json:"line_number" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "bad request", taskID)
		return
	}
	thread, err := h.svc.Open(
		c.Request.Context(),
		taskID,
		body.File,
		body.Line,
		body.Content,
	)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), taskID)
		return
	}
	c.JSON(http.StatusCreated, libs.DataResponse(thread))
}

// PostMessage handles POST /threads/:id/messages.
func (h *Handlers) PostMessage(
	c *gin.Context,
) {
	threadID := c.Param("id")
	var body struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "bad request", threadID)
		return
	}
	if err := h.svc.PostMessage(c.Request.Context(), threadID, "human", body.Content); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", threadID)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), threadID)
		return
	}
	c.Status(http.StatusNoContent)
}

// ForceApprove handles POST /threads/:id/force-approve.
func (h *Handlers) ForceApprove(
	c *gin.Context,
) {
	threadID := c.Param("id")
	if err := h.svc.ForceApprove(c.Request.Context(), threadID); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", threadID)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), threadID)
		return
	}
	thread, err := h.svc.Get(c.Request.Context(), threadID)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), threadID)
		return
	}
	libs.WriteQueryOK(c, thread)
}
