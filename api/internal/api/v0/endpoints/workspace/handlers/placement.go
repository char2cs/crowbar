package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

type placeWorkspaceRequest struct {
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

type workspacePlacementDTO struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

type placeWorkspaceResponse struct {
	Workspace workspacePlacementDTO `json:"workspace"`
	Shifted   []dto.AgentChatDTO    `json:"shifted"`
}

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
