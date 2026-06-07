package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// Diagnostics handles GET /v0/workspaces/:wsId/lsp/diagnostics. The data field
// carries the latest diagnostics snapshot for the workspace under the
// DiagnosticsEvent shape, empty until diagnostics arrive.
func (h *Handlers) Diagnostics(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	if _, ok := h.worktreePath(c); !ok {
		return
	}
	wsID := c.Param("wsId")
	diags := h.lsp.DiagnosticsSnapshot(wsID)
	libs.WriteQueryOK(
		c,
		domlsp.DiagnosticsEvent{
			WsID:        wsID,
			Diagnostics: diags,
		},
	)
}
