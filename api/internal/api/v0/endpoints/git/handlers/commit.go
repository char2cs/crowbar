package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// commitRequest is the POST commit body: the required subject line and an
// optional body.
type commitRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Commit handles POST /v0/workspaces/:wsId/git/commit, recording the staged
// changes with the given subject and optional body. The subject is required; a
// missing subject yields 400. The response echoes the workspace id.
func (h *Handlers) Commit(
	c *gin.Context,
) {
	var body commitRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Subject == "" {
		libs.WriteErr(c, http.StatusBadRequest, "subject is required")
		return
	}
	err := h.git.Commit(c.Request.Context(), c.Param("wsId"), body.Subject, body.Body, h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
