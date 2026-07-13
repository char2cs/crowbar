package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Providers handles GET .../workspaces/:wsId/agent/providers: the registered
// agent providers (id + display name + inline SVG icon) that back the chat row
// glyph, the New-chat rows, and the provider-switch menu. The :wsId path param is
// only used to resolve crowbar home for on-disk descriptor overrides — the
// provider set itself is workspace-independent (kept on the workspace group for
// surface consistency, 00 agentic-engine spec §7.2).
func (h *Handlers) Providers(
	ctx *gin.Context,
) {
	descs, err := h.usecase.ListProviders(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	out := make([]dto.AgentProviderDTO, 0, len(descs))
	for _, d := range descs {
		out = append(out, dto.AgentProviderDTO{ID: d.ID, DisplayName: d.DisplayName, Icon: d.Icon})
	}
	libs.WriteQueryOK(ctx, out)
}
