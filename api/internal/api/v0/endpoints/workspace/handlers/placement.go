package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// placeWorkspaceRequest is the PATCH .../workspaces/:wsId/placement body —
// the workspace half of the same gesture .../chats/:id/placement already
// serves for a chat or a folder row (folders.go's placeChatRequest).
type placeWorkspaceRequest struct {
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// workspacePlacementDTO is a locked branch's own sidebar row as this route
// answers it: no conversation, no runtime — workspaceAnchorView (tree
// package) carries no more than this either. Deliberately not dto.AgentChatDTO:
// the subject here is never a real Chat aggregate, and rendering it as one
// would promise fields (title, provider, working, ...) this row does not have.
type workspacePlacementDTO struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

// placeWorkspaceResponse is the body of PATCH .../workspaces/:wsId/placement:
// the moved branch's own row, and the FOLDER rows the densify shifted
// alongside it — the same shifted-siblings contract placeChatResponse
// carries (folders.go), for the same reason: a drop renumbers a whole
// sibling level, and a client told only about the row it dragged holds stale
// orders for every sibling until its next reconnect.
//
// A shifted REPO or WORKSPACE-ANCHOR sibling is absent here on the same
// terms persist's own doc already discloses for a repo-phantom sibling
// (plan.go): its write is a Node command, not a Chat aggregate command, so it
// carries no hub-projection broadcast of its own and persist's own `written`
// filter never includes it — a real, pre-existing, disclosed gap in what
// THIS HTTP response can report back to the caller that dragged it. The
// LIVE-UPDATE half of that same gap (caught live, 2026-09-09: a locked
// branch dragged past a sibling PATCHed 200 and never moved on screen) is
// closed in PlaceWorkspace itself, which now announces the moved branch's
// own row and every FOLDER row here on the chats WS regardless of what this
// struct can carry back in the response body.
type placeWorkspaceResponse struct {
	Workspace workspacePlacementDTO `json:"workspace"`
	Shifted   []dto.AgentChatDTO    `json:"shifted"`
}

// PlaceWorkspace handles PATCH .../workspaces/:wsId/placement: where a
// LOCKED branch's own row sits in its repo's tree, and where among its
// siblings — addressed by the workspace's own id, never by the chat that
// owns its worktree (that chat still exists and still owns lock/sync/merge/
// reparent/etc., see worktree/routes.go; only this ONE fact — the branch's
// own sidebar position — moved off it, because it is a fact about the
// branch, not about any conversation inside it).
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	// The moved branch's own row, and every FOLDER row the densify shifted —
	// see placeWorkspaceResponse's own doc: neither used to be announced at
	// all, this route's write being a Node command with no hub projection of
	// its own (same gap PlaceChat's identical call already closes for a
	// chat's own drag).
	for _, row := range shifted {
		h.broadcastFolder(row.ID, wsID, "folder_updated")
	}
	h.broadcastFolder(placed.ID, wsID, "placement_set")
	libs.WriteQueryOK(ctx, placeWorkspaceResponse{
		Workspace: workspacePlacementDTO{ID: placed.ID, ParentID: placed.ParentID, Order: placed.Order},
		Shifted:   dto.AgentChatDTOList(shifted, nil, nil),
	})
}
