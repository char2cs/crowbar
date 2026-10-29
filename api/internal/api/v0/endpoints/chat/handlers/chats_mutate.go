package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Rename handles POST .../repos/:repoId/chats/:id/rename: sets the
// chat's title. `?source=agent` applies the agent precedence rule (skip if
// user-locked); the default (a human/FE rename) sets unconditionally and
// locks. 404s (via requireChatInWorkspace) on an unknown id.
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	source := ctx.Query("source")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.chats.RenameChat(rctx, id, body.Title, source); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteAccepted(ctx)
}

// Promote handles POST .../repos/:repoId/chats/:id/promote: fills a bubble's
// empty workspace slot with a worktree forked from its resolved fork parent and
// respawns its current provider there (model spec §4.2, wire contract §5.1).
//
// It responds with the promoted chat itself rather than an Accepted, because
// the one thing the caller asked about — which workspace the row now owns — is
// only knowable from the answer: the workspace is minted server-side, and the
// row's own walks (ownsWorktree, workspaceId) change with it.
//
// It is a POST on the chat and not a PATCH of its workspaceId for the reason
// the model spec gives promotion its own verb: this is not a field write. It
// cuts a branch, adds a worktree, tears the running CLI down and brings a new
// one up in the new directory — and it is one-way (§4.2, "a worktree is never
// demoted"). 404s (via requireChatInWorkspace) on an unknown id; 409s on a
// chat that already has one.
func (h *Handlers) Promote(
	ctx *gin.Context,
) {
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	promoted, err := h.chats.Promote(ctx.Request.Context(), id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	_, rt, err := h.chatSnapshot(ctx.Request.Context(), promoted.ID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(
		promoted, rt, h.chatWorktree(ctx.Request.Context(), promoted)))
}

// SetSelection handles PATCH .../repos/:repoId/chats/:id/selection:
// writes the chat's sticky choice of model and reasoning effort.
//
// The body is the WHOLE selection, not a patch of one field: an omitted or empty
// value clears that half back to the provider's own default. The two move
// together because they are not independent — which effort levels are valid is a
// property of the model — so a partial write could store a pair that was never
// jointly valid.
//
// 404s (via requireChatInWorkspace) on an unknown id; 400s when a value is
// outside the provider's declared catalogue.
func (h *Handlers) SetSelection(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.chats.SetChatSelection(rctx, id, body.Model, body.Effort); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteAccepted(ctx)
}

// Delete handles DELETE .../repos/:repoId/chats/:id: hard-deletes the
// chat AND EVERY CHAT THREADED BELOW IT, each through PurgeChat (best-effort PTY
// teardown, then asynx Forget), plus any folder caught inside that subtree. Each
// chat's event log is erased, not merely tombstoned, so it is gone from every
// subsequent read, including a direct GetChat by id.
//
// The cascade is the panel's rule and it is the OPPOSITE of deleting a folder,
// which promotes what it held. A thread is not filed under its parent, it
// CONTINUES it — it reads that chat's turns whenever it asks — so promoting it
// would leave a conversation whose entire premise has been deleted and which no
// drag can restore. That is why this goes through the tree usecase rather than
// straight to PurgeChat: only something holding the tree knows which chats those
// are.
//
// The scoped "deleted" broadcast every client sees for each purged chat comes
// from the hub projection's OnForget (Task 5), not from here; the folders taken
// with them have no projection to ride, so those frames ARE sent from here,
// keyed on :wsId when the request still has one (the home mount) and "" — every
// subscriber, since the WS filter degrades to unfiltered with no :wsId bound —
// otherwise. 404s (via requireChatInWorkspace) on an unknown id.
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	wsID := ctx.Param("wsId")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	removed, err := h.folders.DeleteChat(rctx, id, libs.DeleteConsentOf(ctx))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	for _, folderID := range removed.Folders {
		h.broadcastFolder(folderID, wsID, "folder_deleted")
	}
	h.announceFolders(wsID, removed.Shifted, "folder_updated")
	libs.WriteAccepted(ctx)
}
