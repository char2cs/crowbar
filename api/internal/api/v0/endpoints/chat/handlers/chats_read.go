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

// List handles GET .../chats: every conversation chat with its runner facts, so the list renders
// provider glyphs and attaches panes without a round trip per row. Scoped to the home mount's :wsId
// when present, else to the repo each row's cwd walk lands in.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	// Git resolves each worktree's merge overlay while the reads below run.
	worktrees := h.beginWorktrees(rctx, ctx.Param("projectId"), ctx.Param("repoId"), nil)
	defer worktrees.done()

	chats, runtimes, err := h.chatsWithRuntimes(rctx, ctx.Param("wsId"), ctx.Param("repoId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(chats, runtimes, worktrees.rowFn(rctx, chats)))
}

// chatsWithRuntimes is the chats List serves and the snapshot each renders from.
func (h *Handlers) chatsWithRuntimes(
	ctx context.Context,
	wsID string,
	repoID string,
) ([]domain.Chat, map[string]dto.ChatRuntime, error) {
	chats, err := h.listChats(ctx, wsID, repoID)
	if err != nil {
		return nil, nil, err
	}
	// Every row is its versioned snapshot — the same answer, from the same
	// in-memory owner, the chat feed's frames carry. No per-row query.
	ids := make([]string, len(chats))
	for i, c := range chats {
		ids[i] = c.ID
	}
	snaps, err := h.chats.ChatSnapshotsOf(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	// A chat deleted since it was listed has no snapshot and leaves the list.
	chats = chats[:0]
	runtimes := make(map[string]dto.ChatRuntime, len(snaps))
	for _, s := range snaps {
		chat, runtime := snapshotParts(s)
		chats, runtimes[chat.ID] = append(chats, chat), runtime
	}
	return chats, runtimes, nil
}

// listChats scopes to wsID when set, else repoID. A request naming neither (no mounted route can) gets
// nothing, never every chat the daemon knows.
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

// Get handles GET .../chats/:id: the chat, its runner facts and the conversations it has hosted.
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
		s.TerminalWait, s.AttachedSessionID, conversation, s.Ledger.Busy)
}

// requireChatInWorkspace loads chatID and holds it to the scope of the mount it was reached through,
// 404ing otherwise: the home mount compares the stored workspace, the repo mount the chat's ground repo.
// ok is false when a response was already written. The messages differ, so existence is not hidden.
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

// chatGround resolves the workspace a chat runs on: its own, or a bubble's cwd-walk result. !ok means
// "no answer", and the caller lets it through: 404ing an unscoped row or a failed read would make the
// client's vanished-chat recovery delete it.
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
