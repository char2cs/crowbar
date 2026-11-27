package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (h *Handlers) Get(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	ws.Working = h.working.WorkingFor(ws.ID)
	owningChatID := h.resolveOwningChatID(c.Request.Context(), ws)
	libs.WriteQueryWithStatus(c, http.StatusOK,
		dto.WorkspaceDTOFrom(c.Request.Context(), ws, wsrepo.MergeEligibility{}, owningChatID, nil))
}

func (h *Handlers) resolveOwningChatID(
	ctx context.Context,
	ws domain.Workspace,
) string {
	if h.owners != nil {
		return h.owners.OwnerOf(ctx, ws)
	}
	if h.chats == nil {
		return ""
	}
	rows, err := h.chats.ListChatsByWorkspace(ctx, ws.ID)
	if err != nil {
		return ""
	}
	owner, ok := domain.ResolveOwningChat(rows)
	if !ok {
		return ""
	}
	return owner.ID
}
