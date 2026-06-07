package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// Search handles POST /v0/workspaces/:wsId/search, walking the workspace
// worktree for the query and returning the match list under the data envelope.
// A missing or empty query is a 400; an absent engine is a 503; an unknown
// workspace is a 404.
func (h *Handlers) Search(
	c *gin.Context,
) {
	if !h.requireEngine(c) {
		return
	}
	ws, ok := h.workspace(c)
	if !ok {
		return
	}
	var body dto.SearchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}
	resp, err := h.eng.Search(
		c.Request.Context(),
		ws.WorktreePath,
		enginesearch.SearchRequest{
			Query:         body.Query,
			CaseSensitive: body.CaseSensitive,
			WholeWord:     body.WholeWord,
			Regex:         body.Regex,
			Include:       body.Include,
			Exclude:       body.Exclude,
		},
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.SearchResponseToDTO(resp))
}
