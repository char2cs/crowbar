package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// Replace handles POST /v0/workspaces/:wsId/search/replace, rewriting matching
// occurrences on disk. The workspace's Locked flag is forwarded to the engine
// so a provider-protected workspace rejects the write with a 409. A file scope
// resolving outside the worktree or an invalid pattern is a 400; an absent
// engine is a 503; an unknown workspace is a 404. Success carries an empty data
// envelope.
func (h *Handlers) Replace(
	c *gin.Context,
) {
	if !h.requireEngine(c) {
		return
	}
	ws, ok := h.workspace(c)
	if !ok {
		return
	}
	var body dto.ReplaceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}
	err := h.eng.Replace(
		c.Request.Context(),
		ws.WorktreePath,
		enginesearch.ReplaceRequest{
			Query:         body.Query,
			Replacement:   body.Replacement,
			Scope:         body.Scope,
			CaseSensitive: body.CaseSensitive,
			WholeWord:     body.WholeWord,
			Regex:         body.Regex,
		},
		ws.Locked,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusOK, nil)
}
