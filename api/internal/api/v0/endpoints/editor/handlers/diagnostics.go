package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func (h *Handlers) Diagnostics(
	c *gin.Context,
) {
	if !h.requireLSP(c) {
		return
	}
	if _, ok := h.worktreePath(c); !ok {
		return
	}
	ownerID := h.lspOwnerID(c)
	diags := h.lsp.DiagnosticsSnapshot(ownerID)
	libs.WriteQueryOK(
		c,
		domlsp.DiagnosticsEvent{
			WsID:        ownerID,
			Diagnostics: diags,
		},
	)
}
