package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

const pollTimeout = 10 * time.Second

// ProviderState handles GET /v0/workspaces/:wsId/provider, running an immediate
// poll for the workspace and returning its provider state: the protected flag
// and the current pull request, nil when the branch has none. A disabled
// capability still yields a 200 envelope carrying { protected: false }. The
// poll is bounded by a 10s timeout.
func (h *Handlers) ProviderState(
	c *gin.Context,
) {
	if !h.requireProvider(c) {
		return
	}
	ws, ok := h.workspace(c, "wsId")
	if !ok {
		return
	}
	pollCtx, cancel := context.WithTimeout(
		c.Request.Context(),
		pollTimeout,
	)
	defer cancel()
	state, err := h.provider.PollOnView(
		pollCtx,
		ws.ID,
		ws.WorktreePath,
		ws.Branch,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ProviderStateDTOFrom(state))
}
