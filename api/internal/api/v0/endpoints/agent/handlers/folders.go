package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agentchatfolder"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// createFolderRequest is the POST .../agent/folders body.
type createFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}

// patchFolderRequest is the PATCH .../agent/folders/:folderId body. Every field
// is optional and a nil field is left as it is, so a rename, a re-parent and a
// reorder are the same endpoint — which is what a drag that does two of them at
// once needs.
type patchFolderRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// placeChatRequest is the PATCH .../agent/chats/:id/placement body. It is the
// chat half of the same gesture the folder PATCH serves, minus the name: a chat
// is renamed through its own rename route, which carries the agent/user title
// precedence this endpoint has no business in.
type placeChatRequest struct {
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// folderResponse is the body of every chat-folder mutation: the row the caller
// asked about, plus the OTHER folder rows the dense renumber moved.
//
// shifted is not a convenience. Chats and folders share one sibling space, so a
// single drop renumbers a whole level; a client told only about the row it
// dragged holds stale orders for every sibling until its next reconnect, and
// draws them in the wrong sequence in the meantime.
type folderResponse struct {
	Folder  dto.AgentChatFolderDTO   `json:"folder"`
	Shifted []dto.AgentChatFolderDTO `json:"shifted"`
}

// deleteFolderResponse is the body of DELETE .../agent/folders/:folderId. There
// is no folder to return — it is gone — but the rows its children's promotion
// renumbered still have to reach the caller.
type deleteFolderResponse struct {
	Shifted []dto.AgentChatFolderDTO `json:"shifted"`
}

// placeChatResponse is the body of PATCH .../agent/chats/:id/placement: the
// moved chat, and the folder rows the densify shifted. The chats that shifted
// alongside it are absent on purpose — their write is an aggregate command, so
// each one has already announced itself on the Chats socket.
type placeChatResponse struct {
	Chat    dto.AgentChatDTO         `json:"chat"`
	Shifted []dto.AgentChatFolderDTO `json:"shifted"`
}

// ListFolders handles GET .../agent/folders, returning the workspace's chat
// folders as AgentChatFolderDTO[] in panel order. It is the read a reconnect
// reseeds from: the Chats socket carries no snapshot, so this list is the only
// full answer there is.
func (h *Handlers) ListFolders(
	ctx *gin.Context,
) {
	rows, err := h.folders.ListInWorkspace(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, dto.AgentChatFolderDTOList(rows))
}

// CreateFolder handles POST .../agent/folders. The URL scope is authoritative: a
// body-supplied workspace would let a POST against one workspace create a folder
// in another, so none is accepted.
func (h *Handlers) CreateFolder(
	ctx *gin.Context,
) {
	var body createFolderRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	wsID := ctx.Param("wsId")
	created, shifted, err := h.folders.Create(ctx.Request.Context(), agentchatfolder.CreateInput{
		WorkspaceID: wsID,
		ParentID:    body.ParentID,
		Name:        body.Name,
	})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, shifted, "folder_updated")
	h.announceFolders(wsID, []domain.AgentChatFolder{created}, "folder_created")
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, folderResponse{
		Folder:  dto.AgentChatFolderDTOFrom(created),
		Shifted: dto.AgentChatFolderDTOList(shifted),
	})
}

// PatchFolder handles PATCH .../agent/folders/:folderId: rename, re-parent and
// reorder, in that order, so a single drag that renames and moves lands as one
// answer rather than two half-states.
func (h *Handlers) PatchFolder(
	ctx *gin.Context,
) {
	var body patchFolderRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	wsID := ctx.Param("wsId")
	updated, shifted, err := h.applyFolderPatch(ctx.Request.Context(), wsID, ctx.Param("folderId"), body)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, append(shifted, updated), "folder_updated")
	libs.WriteQueryOK(ctx, folderResponse{
		Folder:  dto.AgentChatFolderDTOFrom(updated),
		Shifted: dto.AgentChatFolderDTOList(shifted),
	})
}

// applyFolderPatch runs the rename first and the placement second. A PATCH that
// carries neither still goes through Move, which is a no-op placement whose only
// job here is to return the row's real state.
func (h *Handlers) applyFolderPatch(
	ctx context.Context,
	wsID string,
	id string,
	body patchFolderRequest,
) (domain.AgentChatFolder, []domain.AgentChatFolder, error) {
	if body.Name != nil {
		if _, err := h.folders.Rename(ctx, wsID, id, *body.Name); err != nil {
			return domain.AgentChatFolder{}, nil, err
		}
	}
	return h.folders.Move(ctx, wsID, id, agentchatfolder.MoveInput{ParentID: body.ParentID, Order: body.Order})
}

// DeleteFolder handles DELETE .../agent/folders/:folderId. What the folder held
// is PROMOTED to the folder's own parent, never deleted: a folder holds no
// conversation, so removing the chats filed under it would destroy work the user
// only meant to unfile. That is the opposite of deleting a CHAT, which does take
// its subtree — see DeleteChat.
func (h *Handlers) DeleteFolder(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")
	id := ctx.Param("folderId")
	promoted, err := h.folders.Delete(ctx.Request.Context(), wsID, id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, promoted, "folder_updated")
	h.broadcastFolder(id, wsID, "folder_deleted")
	libs.WriteQueryOK(ctx, deleteFolderResponse{Shifted: dto.AgentChatFolderDTOList(promoted)})
}

// PlaceChat handles PATCH .../agent/chats/:id/placement: where the chat hangs in
// the tree and where it sits among its siblings.
//
// Unlike the sidebar's equivalent this write DOES move lineage, and deliberately.
// A chat's parent is not a fact about anything on disk — it IS the relationship,
// and the panel draws no other mark for it — so dragging a chat under another
// chat makes it a thread that reads that chat's turns, and dragging it back out
// makes it standalone again.
func (h *Handlers) PlaceChat(
	ctx *gin.Context,
) {
	var body placeChatRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	wsID := ctx.Param("wsId")
	placed, shifted, err := h.folders.PlaceChat(ctx.Request.Context(), wsID, ctx.Param("id"),
		agentchatfolder.PlaceInput{ParentID: body.ParentID, Order: body.Order})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, shifted, "folder_updated")
	rt, err := h.chatRuntime(ctx.Request.Context(), placed.ID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, placeChatResponse{
		Chat:    dto.AgentChatDTOFrom(placed, rt),
		Shifted: dto.AgentChatFolderDTOList(shifted),
	})
}

// announceFolders fans one frame per folder row a mutation wrote out on the
// Chats socket. The collateral matters as much as the subject: a move renumbers
// the levels it left and joined, and a client told only about the row that was
// dragged would hold stale orders for its siblings until the next reconnect.
func (h *Handlers) announceFolders(
	wsID string,
	rows []domain.AgentChatFolder,
	kind string,
) {
	for _, row := range rows {
		h.broadcastFolder(row.ID, wsID, kind)
	}
}
