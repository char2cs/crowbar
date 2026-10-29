package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// List handles GET .../repos/:repoId/chats, returning every conversation-typed
// chat, each carrying the runner facts derived for it by chatRuntime (its live
// runner, that runner's PTY, its provider) — so the chat list can render
// provider glyphs and attach a pane without a second round trip per row.
//
// A request that still names a workspace (the home mount's injected :wsId)
// scopes to it exactly as before. Otherwise it scopes to the REPO in the URL:
// each row's owning workspace is resolved through the cwd walk and its repo
// compared against :repoId, so a bubble is listed under the repo it actually
// runs in and no chat from another repo is served at all.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	// Git resolves each worktree's merge overlay while the reads below run.
	worktrees := h.beginWorktrees(rctx, ctx.Param("projectId"), ctx.Param("repoId"), nil)
	defer worktrees.done()

	chats, err := h.listChats(rctx, ctx.Param("wsId"), ctx.Param("repoId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	// Every row is its versioned snapshot — the same answer, from the same
	// in-memory owner, the chat feed's frames carry. No per-row query.
	ids := make([]string, len(chats))
	for i, c := range chats {
		ids[i] = c.ID
	}
	snaps, err := h.chats.ChatSnapshotsOf(rctx, ids)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	runtimes := make(map[string]dto.ChatRuntime, len(chats))
	for i, s := range snaps {
		chats[i], runtimes[ids[i]] = snapshotParts(s)
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(chats, runtimes, worktrees.rowFn(rctx, chats)))
}

// listChats backs List: wsID scopes to ListChatsByWorkspace when the request
// still names a workspace (the home mount's injected :wsId), otherwise repoID
// scopes to ListChatsInRepo — the walk that resolves each row's owning
// workspace and compares its repo against the one in the URL.
//
// A request naming NEITHER cannot happen on any mounted route (the repo group
// always binds :repoId, the home group always injects :wsId), and is answered
// with nothing rather than with every chat the daemon knows: an unscoped list
// is what this route used to serve by accident, and it is not a scope any
// caller should be able to ask for.
func (h *Handlers) listChats(
	ctx context.Context,
	wsID string,
	repoID string,
) ([]domain.Chat, error) {
	if wsID != "" {
		return h.chats.ListChatsByWorkspace(ctx, wsID)
	}
	if repoID == "" {
		return nil, nil
	}
	return h.chats.ListChatsInRepo(ctx, repoID)
}

// Get handles GET .../repos/:repoId/chats/:id, returning the chat with its
// derived runner facts and the conversations it has hosted — the append-only history
// that succeeds the deleted segment list. 404s (via requireChatInWorkspace) when id
// names an unknown chat, or (for a request that still names a workspace) one
// anchored to a DIFFERENT workspace.
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}

	snap, rt, err := h.chatSnapshot(ctx.Request.Context(), chat.ID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	rt.Conversations, err = h.runners.ConversationsForChat(ctx.Request.Context(), chat.ID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(
		snap, rt, h.chatWorktree(ctx.Request.Context(), snap)))
}

// chatSnapshot is chatID's versioned snapshot, split into the chat row and its
// runtime for the DTO. It is the ONE source of every chat body this API serves,
// the same one every chat frame carries.
func (h *Handlers) chatSnapshot(
	ctx context.Context,
	chatID string,
) (domain.Chat, dto.ChatRuntime, error) {
	s, err := h.chats.ChatSnapshot(ctx, chatID)
	if err != nil {
		return domain.Chat{}, dto.ChatRuntime{}, err
	}
	chat, rt := snapshotParts(s)
	return chat, rt, nil
}

// snapshotParts splits a snapshot into the chat row and its runtime.
func snapshotParts(s agentusecase.ChatSnapshot) (domain.Chat, dto.ChatRuntime) {
	conversation, _ := s.Conversation()
	return s.Chat, dto.ChatSnapshotRuntime(s.Live, s.Phase, s.Version,
		s.TerminalWait, s.AttachedSessionID, s.Session, conversation, s.Ledger.Busy)
}

// requireChatInWorkspace loads chatID, 404ing on an unknown id, and holds it to
// the scope of the mount it was reached through — EVERY mount, not only the one
// that happens to bind a workspace.
//
// The home mount injects :wsId (RequireHomeWorkspace) and a project home's chats
// stay workspace-scoped, so there the test is the stored workspace, exactly as
// before Task 17 — and exactly what the home list (ListChatsByWorkspace) is.
//
// The repo mount binds :repoId and no workspace at all. Addressing a chat by id
// alone (model spec §5.1) retired the WORKSPACE comparison there and it stays
// retired — any of the repo's own workspaces is legitimate — but it never
// retired the REPO boundary, and nothing re-asserted it: every chat id in the
// daemon resolved through a repo mount, project-home chats included, which no
// repo's list will ever mention. So the repo mount compares the chat's GROUND
// against :repoId (chatGround), the same membership ListChatsInRepo computes
// for the list beside it.
//
// A request naming neither cannot happen on any mounted route (the repo group
// always binds :repoId, the home group always injects :wsId) and is left alone,
// the same posture listChats takes on the same impossible input.
//
// It is the by-id scope check shared by Get/Switch/Rename/Handoff (and
// Delete, Task 5): every one of those routes takes a bare chat id with no
// other scoping input. The unknown-id and wrong-scope cases both return
// HTTP 404 (never the chat body), so no out-of-scope chat is ever served
// to the caller; the two responses carry DIFFERENT body
// messages ("chat not found in workspace" vs the mapped GetChat not-found
// text), so a probe can still tell "exists elsewhere" from "does not exist" —
// an accepted minor, the scope check's job is to deny cross-scope ACCESS,
// not to perfectly hide existence. ok is false exactly when the caller must
// return immediately because a response was already written (either this 404
// or a mapped GetChat error).
func (h *Handlers) requireChatInWorkspace(
	ctx *gin.Context,
	chatID string,
) (domain.Chat, bool) {
	chat, err := h.chats.GetChat(ctx.Request.Context(), chatID)
	if err != nil {
		libs.Fail(ctx, err)
		return domain.Chat{}, false
	}
	if !h.chatInMountScope(ctx, chat) {
		libs.WriteErr(ctx, http.StatusNotFound, "chat not found in workspace")
		return domain.Chat{}, false
	}
	return chat, true
}

// chatInMountScope answers whether chat may be addressed through THIS mount.
func (h *Handlers) chatInMountScope(
	ctx *gin.Context,
	chat domain.Chat,
) bool {
	if wsID := ctx.Param("wsId"); wsID != "" {
		return chat.WorkspaceID == wsID
	}
	repoID := ctx.Param("repoId")
	if repoID == "" || h.worktrees == nil {
		return true
	}
	ground, ok := h.chatGround(ctx.Request.Context(), chat)
	if !ok {
		return true
	}
	return ground.RepoID == repoID
}

// chatGround resolves the workspace a chat RUNS on: its own, or — for a bubble,
// which carries none — the one its cwd walk lands on (model spec §3.2). A
// resolved workspace's RepoID is a WHOLE answer even when empty: that is
// exactly what a project-home workspace is.
//
// !ok means "no answer", never "no repo", and the caller must wave it through.
// A row whose whole ancestry owns no workspace has no scope to be held to (the
// posture matchScopeOrUnscoped takes on the socket) and a failed read is not a
// scope verdict; 404ing either would hand the client's vanished-chat recovery
// the very deletion this check exists to prevent. It is the one asymmetry with
// ListChatsInRepo, which merely drops such a row from a list.
func (h *Handlers) chatGround(
	ctx context.Context,
	chat domain.Chat,
) (domain.Workspace, bool) {
	workspaceID := chat.WorkspaceID
	if workspaceID == "" {
		resolved, ok, err := h.chats.CwdWorkspaceID(ctx, chat.ID)
		if err != nil || !ok {
			return domain.Workspace{}, false
		}
		workspaceID = resolved
	}
	ws, err := h.worktrees.Get(ctx, workspaceID)
	if err != nil {
		return domain.Workspace{}, false
	}
	return ws, true
}
