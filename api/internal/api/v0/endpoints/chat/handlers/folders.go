package handlers

import (
	"context"
	"net/http"
	"slices"
	"strings"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// createFolderRequest is the POST .../chats/folders body.
type createFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}

// patchFolderRequest is the PATCH .../chats/folders/:folderId body. A nil field is left as is, so one
// drag that renames and moves at once is a single request.
type patchFolderRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// placeChatRequest is the PATCH .../chats/:id/placement body: the folder PATCH minus the name, since a
// chat renames through its own route with its agent/user title precedence.
type placeChatRequest struct {
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// folderResponse is the body of every folder mutation: the row asked about plus every other row the
// dense renumber moved. Chats and folders share one sibling space, so omitting shifted rows would leave
// a client drawing a whole level out of order until it reconnects.
type folderResponse struct {
	Folder  dto.AgentChatDTO   `json:"folder"`
	Shifted []dto.AgentChatDTO `json:"shifted"`
}

// deleteFolderResponse is the body of DELETE .../chats/folders/:folderId. There
// is no folder to return — it is gone — but the rows its children's promotion
// renumbered still have to reach the caller.
type deleteFolderResponse struct {
	Shifted []dto.AgentChatDTO `json:"shifted"`
}

// placeChatResponse is the PATCH .../chats/:id/placement body. Shifted carries only folder rows: the
// shifted chats' writes are aggregate commands that already announced themselves on the Chats socket.
type placeChatResponse struct {
	Chat    dto.AgentChatDTO   `json:"chat"`
	Shifted []dto.AgentChatDTO `json:"shifted"`
}

// folderDTOList renders rows sorted by (order, id) within each parent, the one converter every read
// uses so lists and reseeds agree. It resolves worktree fields too: a shifted row can be a
// worktree-owning chat, which must not lose its branch and diff counts.
func (h *Handlers) folderDTOList(
	ctx *gin.Context,
	rows []domain.Chat,
) []dto.AgentChatDTO {
	return h.foldersIn(ctx.Request.Context(), ctx.Param("projectId"), ctx.Param("repoId"), rows)
}

func (h *Handlers) foldersIn(
	ctx context.Context,
	projectID string,
	repoID string,
	rows []domain.Chat,
) []dto.AgentChatDTO {
	worktreeFn := h.repoWorktrees(ctx, projectID, repoID, rows)
	out := dto.AgentChatDTOList(rows, nil, worktreeFn)
	slices.SortFunc(out, compareFolderDTOs)
	return out
}

func compareFolderDTOs(
	a dto.AgentChatDTO,
	b dto.AgentChatDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}

// listFolders is a repo's folders, or with no repo the home workspace's.
func (h *Handlers) listFolders(ctx context.Context, repoID, homeID string) ([]domain.Chat, error) {
	if repoID == "" {
		return h.folders.ListInHome(ctx, homeID)
	}
	return h.folders.ListInRepo(ctx, repoID)
}

// ListFolders handles GET .../chats/folders, returning the repo's folders in
// panel order. It is the read a reconnect reseeds from: the Chats socket
// carries no snapshot, so this list is the only full answer there is.
func (h *Handlers) ListFolders(
	ctx *gin.Context,
) {
	rows, err := h.listFolders(ctx.Request.Context(), ctx.Param("repoId"), ctx.Param("wsId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, h.folderDTOList(ctx, rows))
}

// CreateFolder handles POST .../chats/folders. The URL scope names the repo the
// new folder is created in.
func (h *Handlers) CreateFolder(
	ctx *gin.Context,
) {
	var body createFolderRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	wsID := ctx.Param("wsId")
	in := agentusecase.CreateInput{
		RepoID:   ctx.Param("repoId"),
		ParentID: body.ParentID,
		Name:     body.Name,
	}
	if in.RepoID == "" {
		in.HomeID = wsID
	}
	created, shifted, err := h.folders.Create(ctx.Request.Context(), in)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, folderResponse{
		Folder:  dto.AgentChatDTOFrom(created, dto.ChatRuntime{}, nil),
		Shifted: h.folderDTOList(ctx, shifted),
	})
}

// PatchFolder handles PATCH .../chats/folders/:folderId: rename, re-parent and
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
	if !h.folderInScope(ctx) {
		return
	}
	updated, shifted, err := h.applyFolderPatch(ctx.Request.Context(), ctx.Param("folderId"), body)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, folderResponse{
		Folder:  dto.AgentChatDTOFrom(updated, dto.ChatRuntime{}, nil),
		Shifted: h.folderDTOList(ctx, shifted),
	})
}

// applyFolderPatch renames first, then places. A name-only PATCH skips Move, whose guardNotWorking
// would refuse the rename whenever a chat inside the folder is working.
func (h *Handlers) applyFolderPatch(
	ctx context.Context,
	id string,
	body patchFolderRequest,
) (domain.Chat, []domain.Chat, error) {
	if body.Name != nil {
		renamed, err := h.folders.Rename(ctx, id, *body.Name)
		if err != nil {
			return domain.Chat{}, nil, err
		}
		if body.ParentID == nil && body.Order == nil {
			return renamed, nil, nil
		}
	}
	return h.folders.Move(ctx, id, agentusecase.MoveInput{ParentID: body.ParentID, Order: body.Order})
}

// DeleteFolder handles DELETE .../chats/folders/:folderId. Its contents are promoted to its parent,
// never deleted: a folder holds no conversation, so unlike a chat delete it takes no subtree.
func (h *Handlers) DeleteFolder(
	ctx *gin.Context,
) {
	id := ctx.Param("folderId")
	if !h.folderInScope(ctx) {
		return
	}
	promoted, err := h.folders.Delete(ctx.Request.Context(), id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, deleteFolderResponse{Shifted: h.folderDTOList(ctx, promoted)})
}

// PlaceChat handles PATCH .../chats/:id/placement. Unlike the sidebar's move it changes lineage: a
// chat's parent is its thread relationship. With no :wsId, the chat's own workspace is used, since the
// tree usecase asserts the chat belongs to the workspace it is told.
func (h *Handlers) PlaceChat(
	ctx *gin.Context,
) {
	var body placeChatRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	wsID := ctx.Param("wsId")
	if wsID == "" {
		chat, err := h.chats.GetChat(ctx.Request.Context(), ctx.Param("id"))
		if err != nil {
			libs.Fail(ctx, err)
			return
		}
		wsID = chat.WorkspaceID
	}
	placed, shifted, err := h.folders.PlaceChat(ctx.Request.Context(), wsID, ctx.Param("id"),
		agentusecase.PlaceInput{ParentID: body.ParentID, Order: body.Order})
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	_, rt, err := h.chatSnapshot(ctx.Request.Context(), placed.ID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, placeChatResponse{
		Chat:    dto.AgentChatDTOFrom(placed, rt, h.chatWorktree(ctx.Request.Context(), placed)),
		Shifted: h.folderDTOList(ctx, shifted),
	})
}

// folderInScope 404s a :folderId the home mount does not own: every project's home shares the one
// RepoID == "" bucket, so a bare id would reach another project's folder.
func (h *Handlers) folderInScope(
	ctx *gin.Context,
) bool {
	if ctx.Param("repoId") != "" {
		return true
	}
	f, err := h.folders.FolderScope(ctx.Request.Context(), ctx.Param("folderId"))
	if err != nil {
		libs.Fail(ctx, err)
		return false
	}
	if !f.InHome(ctx.Param("wsId")) {
		libs.WriteErr(ctx, http.StatusNotFound, "folder "+f.ID+" is not in this project's home")
		return false
	}
	return true
}
