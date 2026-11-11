package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// placeWorkspaceRequest is the PATCH .../workspaces/:wsId/placement body, the workspace half of
// the gesture .../chats/:id/placement serves for chat and folder rows.
type placeWorkspaceRequest struct {
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// workspacePlacementDTO is a locked branch's own sidebar row. Not dto.AgentChatDTO: the subject is
// never a Chat aggregate, and that shape would promise fields (title, provider, ...) it lacks.
type workspacePlacementDTO struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

// placeWorkspaceResponse is the moved branch's row plus the folder and locked-branch rows the
// densify shifted: a drop renumbers the whole sibling level. Shifted chats and repo headers are
// announced on their own streams.
type placeWorkspaceResponse struct {
	Workspace workspacePlacementDTO `json:"workspace"`
	Shifted   []dto.AgentChatDTO    `json:"shifted"`
}

// PlaceWorkspace handles PATCH .../workspaces/:wsId/placement: where a locked branch's row sits
// in its repo's tree. It is addressed by workspace id, not by the chat owning the worktree, which
// still owns lock/sync/merge/reparent.
func (h *Handlers) PlaceWorkspace(
	ctx *gin.Context,
) {
	var body placeWorkspaceRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	wsID := ctx.Param("wsId")
	placed, shifted, err := h.placer.PlaceWorkspace(ctx.Request.Context(), wsID,
		agentusecase.PlaceInput{ParentID: body.ParentID, Order: body.Order})
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, placeWorkspaceResponse{
		Workspace: workspacePlacementDTO{ID: placed.ID, ParentID: placed.ParentID, Order: placed.Order},
		Shifted:   dto.AgentChatDTOList(shifted, nil, nil),
	})
}
