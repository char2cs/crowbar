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

// patchFolderRequest is the PATCH .../chats/folders/:folderId body. Every field
// is optional and a nil field is left as it is, so a rename, a re-parent and a
// reorder are the same endpoint — which is what a drag that does two of them at
// once needs.
type patchFolderRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parentId"`
	Order    *int    `json:"order"`
}

// placeChatRequest is the PATCH .../chats/:id/placement body. It is the
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
//
// Both ride AgentChatDTO now: a folder is a Chat row like any other (Type ==
// "folder"), so its wire shape is the same one a conversation's placement
// already carries — see PlaceChat below, which has rendered rows this way all
// along.
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

// placeChatResponse is the body of PATCH .../chats/:id/placement: the
// moved chat, and the folder rows the densify shifted. The chats that shifted
// alongside it are absent on purpose — their write is an aggregate command, so
// each one has already announced itself on the Chats socket.
type placeChatResponse struct {
	Chat    dto.AgentChatDTO   `json:"chat"`
	Shifted []dto.AgentChatDTO `json:"shifted"`
}

// folderDTOList renders folder-typed Chat rows through the same converter a
// conversation's placement already uses, with the zero ChatRuntime: a folder
// never has a runner, so every derived field is honestly empty rather than
// omitted.
//
// The sort lives HERE, in the converter every read goes through, because a
// client that got one order from the list and another from a reseed would
// watch its panel reshuffle on every reconnect. Order is only meaningful
// WITHIN a parent, so the list is not a single ordered sequence — it is every
// level's sequence, interleaved; the client groups by parentId and reads each
// group in this order.
//
// It resolves the worktree enrichment despite its name, because a SHIFTED row
// is not always a folder: a densify renumbers every row at the affected level,
// and those levels interleave folders with worktree-owning chats. Serializing
// one of those without its git fields would hand a client a complete-looking
// chat whose branch and diff counts had vanished, on a response about nothing
// but sort order.
func (h *Handlers) folderDTOList(
	ctx *gin.Context,
	rows []domain.Chat,
) []dto.AgentChatDTO {
	worktreeFn := h.repoWorktrees(
		ctx.Request.Context(), ctx.Param("projectId"), ctx.Param("repoId"))
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

// ListFolders handles GET .../chats/folders, returning the repo's folders in
// panel order. It is the read a reconnect reseeds from: the Chats socket
// carries no snapshot, so this list is the only full answer there is.
func (h *Handlers) ListFolders(
	ctx *gin.Context,
) {
	rows, err := h.folders.ListInRepo(ctx.Request.Context(), ctx.Param("repoId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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
	created, shifted, err := h.folders.Create(ctx.Request.Context(), agentusecase.CreateInput{
		RepoID:   ctx.Param("repoId"),
		ParentID: body.ParentID,
		Name:     body.Name,
	})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, shifted, "folder_updated")
	h.announceFolders(wsID, []domain.Chat{created}, "folder_created")
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
	wsID := ctx.Param("wsId")
	updated, shifted, err := h.applyFolderPatch(ctx.Request.Context(), ctx.Param("folderId"), body)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, append(shifted, updated), "folder_updated")
	libs.WriteQueryOK(ctx, folderResponse{
		Folder:  dto.AgentChatDTOFrom(updated, dto.ChatRuntime{}, nil),
		Shifted: h.folderDTOList(ctx, shifted),
	})
}

// applyFolderPatch runs the rename first and the placement second. A PATCH that
// carries neither still goes through Move, which is a no-op placement whose only
// job here is to return the row's real state.
func (h *Handlers) applyFolderPatch(
	ctx context.Context,
	id string,
	body patchFolderRequest,
) (domain.Chat, []domain.Chat, error) {
	if body.Name != nil {
		if _, err := h.folders.Rename(ctx, id, *body.Name); err != nil {
			return domain.Chat{}, nil, err
		}
	}
	return h.folders.Move(ctx, id, agentusecase.MoveInput{ParentID: body.ParentID, Order: body.Order})
}

// DeleteFolder handles DELETE .../chats/folders/:folderId. What the folder held
// is PROMOTED to the folder's own parent, never deleted: a folder holds no
// conversation, so removing the chats filed under it would destroy work the user
// only meant to unfile. That is the opposite of deleting a CHAT, which does take
// its subtree — see DeleteChat.
func (h *Handlers) DeleteFolder(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")
	id := ctx.Param("folderId")
	promoted, err := h.folders.Delete(ctx.Request.Context(), id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	h.announceFolders(wsID, promoted, "folder_updated")
	h.broadcastFolder(id, wsID, "folder_deleted")
	libs.WriteQueryOK(ctx, deleteFolderResponse{Shifted: h.folderDTOList(ctx, promoted)})
}

// PlaceChat handles PATCH .../chats/:id/placement: where the chat hangs in
// the tree and where it sits among its siblings.
//
// Unlike the sidebar's equivalent this write DOES move lineage, and deliberately.
// A chat's parent is not a fact about anything on disk — it IS the relationship,
// and the panel draws no other mark for it — so dragging a chat under another
// chat makes it a thread that reads that chat's turns, and dragging it back out
// makes it standalone again.
//
// The tree usecase asserts the moved chat still belongs to the workspace it is
// told, so a request with no :wsId (the repo-scoped mount, Task 17) resolves
// the chat's OWN current workspace via GetChat first — the assertion then
// compares the chat against itself, which is what "no workspace named in the
// URL" has to mean now that WorkspaceID is optional and mutable. The home
// mount's injected :wsId is used as-is, unchanged from before Task 17.
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
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(ctx, status, msg)
			return
		}
		wsID = chat.WorkspaceID
	}
	placed, shifted, err := h.folders.PlaceChat(ctx.Request.Context(), wsID, ctx.Param("id"),
		agentusecase.PlaceInput{ParentID: body.ParentID, Order: body.Order})
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
		Chat:    dto.AgentChatDTOFrom(placed, rt, h.chatWorktree(ctx.Request.Context(), placed)),
		Shifted: h.folderDTOList(ctx, shifted),
	})
}

// announceFolders fans one frame per folder row a mutation wrote out on the
// Chats socket. The collateral matters as much as the subject: a move renumbers
// the levels it left and joined, and a client told only about the row that was
// dragged would hold stale orders for its siblings until the next reconnect.
func (h *Handlers) announceFolders(
	wsID string,
	rows []domain.Chat,
	kind string,
) {
	for _, row := range rows {
		h.broadcastFolder(row.ID, wsID, kind)
	}
}
