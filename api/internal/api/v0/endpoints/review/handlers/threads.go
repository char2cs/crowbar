package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// openThreadBody is the JSON body of the open-thread POST: the diff anchor
// (file path, line number, side) and the first message body.
type openThreadBody struct {
	FilePath   string            `json:"filePath"`
	LineNumber int               `json:"lineNumber"`
	Side       domain.ReviewSide `json:"side"`
	Body       string            `json:"body"`
}

// OpenThread handles POST /v0/workspaces/:wsId/review/threads, opening a new
// review thread anchored to a file location with its first message. The data
// field carries the created ReviewThreadDTO and the status is 201.
func (h *Handlers) OpenThread(
	c *gin.Context,
) {
	var body openThreadBody
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if !validOpenThread(c, body) {
		return
	}
	thread, err := h.review.OpenThread(c.Request.Context(), branchreview.OpenThreadInput{
		WsID:       c.Param("wsId"),
		FilePath:   body.FilePath,
		LineNumber: body.LineNumber,
		Side:       body.Side,
		Body:       body.Body,
	})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryWithStatus(
		c,
		http.StatusCreated,
		dto.ReviewThreadDTOFrom(thread),
	)
}

func validOpenThread(
	c *gin.Context,
	body openThreadBody,
) bool {
	if body.FilePath == "" {
		libs.WriteErr(c, http.StatusBadRequest, "filePath is required")
		return false
	}
	if body.Side == "" {
		libs.WriteErr(c, http.StatusBadRequest, "side is required")
		return false
	}
	if body.Body == "" {
		libs.WriteErr(c, http.StatusBadRequest, "body is required")
		return false
	}
	return true
}

// replyBody is the JSON body of the reply POST: the reply message body.
type replyBody struct {
	Body string `json:"body"`
}

// Reply handles POST /v0/workspaces/:wsId/review/threads/:id/reply, appending a
// reply message to an existing thread. The data field carries the updated
// ReviewThreadDTO.
func (h *Handlers) Reply(
	c *gin.Context,
) {
	var body replyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Body == "" {
		libs.WriteErr(c, http.StatusBadRequest, "body is required")
		return
	}
	thread, err := h.review.Reply(
		c.Request.Context(),
		c.Param("id"),
		body.Body,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ReviewThreadDTOFrom(thread))
}

// setThreadResolvedBody is the JSON body of the resolve PATCH: whether to mark
// the thread resolved. It is a pointer so an explicit false is distinguishable
// from a missing field.
type setThreadResolvedBody struct {
	IsResolved *bool `json:"isResolved"`
}

// SetThreadResolved handles PATCH /v0/workspaces/:wsId/review/threads/:id,
// marking a thread resolved or reopening it. The data field carries the updated
// ReviewThreadDTO.
func (h *Handlers) SetThreadResolved(
	c *gin.Context,
) {
	var body setThreadResolvedBody
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.IsResolved == nil {
		libs.WriteErr(c, http.StatusBadRequest, "isResolved is required")
		return
	}
	thread, err := h.review.SetThreadResolved(
		c.Request.Context(),
		c.Param("id"),
		*body.IsResolved,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ReviewThreadDTOFrom(thread))
}
