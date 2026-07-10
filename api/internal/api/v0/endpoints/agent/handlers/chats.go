package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Create handles POST .../workspaces/:wsId/agent/chats: spawns a fresh
// AgentChat anchored to the :wsId path param and its first AgentSegment,
// launching the provider's vendor CLI in a PTY. It responds with the new
// chat's id; the spawned segment id is not surfaced here (the client reads it
// back via GET .../agent/chats/:id).
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		Provider string `json:"provider"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	chatID, _, err := h.usecase.SpawnChat(rctx, wsID, body.Provider)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, chatID)
}

// List handles GET .../workspaces/:wsId/agent/chats, returning only the
// chats anchored to the :wsId path param.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	chats, err := h.usecase.ListChatsByWorkspace(rctx, wsID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(chats))
}

// Get handles GET .../workspaces/:wsId/agent/chats/:id, returning the chat
// plus its ordered segment history. 404s (via requireChatInWorkspace) when id
// names a chat anchored to a DIFFERENT workspace than :wsId.
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	chat, ok := h.requireChatInWorkspace(ctx, id)
	if !ok {
		return
	}

	segs, err := h.usecase.SegmentsFor(rctx, id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(chat, segs))
}

// requireChatInWorkspace loads chatID and writes a 404 unless it belongs to the
// :wsId path param. It is the by-id scope check shared by Get/Switch/Rename/
// Handoff (and Delete, Task 5): every one of those routes takes a bare chat id
// with no other workspace-scoping input, so without this check a caller in
// workspace A could address a chat anchored to workspace B by id alone. Both the
// unknown-id and wrong-workspace cases return HTTP 404 (never the chat body), so
// no cross-workspace chat is ever served; the two responses carry DIFFERENT body
// messages ("chat not found in workspace" vs the mapped GetChat not-found text),
// so a probe can still tell "exists elsewhere" from "does not exist" — an
// accepted minor, the scope check's job is to deny cross-workspace ACCESS, not to
// perfectly hide existence. ok is false exactly when the caller must return
// immediately because a response was already written (either this 404 or a mapped
// GetChat error).
func (h *Handlers) requireChatInWorkspace(
	ctx *gin.Context,
	chatID string,
) (domain.AgentChat, bool) {
	chat, err := h.usecase.GetChat(ctx.Request.Context(), chatID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return domain.AgentChat{}, false
	}
	if chat.WorkspaceID != ctx.Param("wsId") {
		libs.WriteErr(ctx, http.StatusNotFound, "chat not found in workspace")
		return domain.AgentChat{}, false
	}
	return chat, true
}

// Rename handles POST .../workspaces/:wsId/agent/chats/:id/rename: sets the
// chat's title. `?source=agent` applies the agent precedence rule (skip if
// user-locked); the default (a human/FE rename) sets unconditionally and
// locks. 404s (via requireChatInWorkspace) when id names a chat anchored to a
// DIFFERENT workspace than :wsId.
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

	if err := h.usecase.RenameChat(rctx, id, body.Title, source); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteAccepted(ctx)
}

// Delete handles DELETE .../workspaces/:wsId/agent/chats/:id: hard-deletes
// the chat via usecase.PurgeChat (best-effort active-segment PTY teardown,
// then asynx Forget): the chat's event log itself is erased, not merely
// tombstoned, so it is gone from every subsequent read, including a direct
// GetChat by id. The scoped "deleted" broadcast every client sees comes from
// the hub projection's OnForget (Task 5), not from here. 404s (via
// requireChatInWorkspace) when id names a chat anchored to a DIFFERENT
// workspace than :wsId.
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	if err := h.usecase.PurgeChat(rctx, id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteAccepted(ctx)
}
