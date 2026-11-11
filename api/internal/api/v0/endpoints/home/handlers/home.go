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

// Get handles GET /v0/projects/:projectId/home.
// It returns the home workspace DTO for the project.
func (h *Handlers) Get(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	// The same WorkingFor seam enriches the live broadcast frames, so a read
	// taken mid-turn agrees with the stream and reports working=true.
	ws.Working = h.working.WorkingFor(ws.ID)
	owningChatID := h.resolveOwningChatID(c.Request.Context(), ws)
	// A home workspace has no merge eligibility and no sidebar placement
	// (PlaceWorkspace refuses it), so both stay at their zero values.
	libs.WriteQueryWithStatus(c, http.StatusOK,
		dto.WorkspaceDTOFrom(c.Request.Context(), ws, wsrepo.MergeEligibility{}, owningChatID, nil))
}

// resolveOwningChatID answers the home workspace's owning chat id for the wire
// DTO, through the chat handlers' OwnerOf when wired; an unwired seam degrades
// to "".
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
