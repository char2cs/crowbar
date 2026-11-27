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

func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	worktrees := h.beginWorktrees(rctx, ctx.Param("projectId"), ctx.Param("repoId"), nil)
	defer worktrees.done()

	chats, runtimes, err := h.chatsWithRuntimes(rctx, ctx.Param("wsId"), ctx.Param("repoId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(chats, runtimes, worktrees.rowFn(rctx, chats)))
}

func (h *Handlers) chatsWithRuntimes(
	ctx context.Context,
	wsID string,
	repoID string,
) ([]domain.Chat, map[string]dto.ChatRuntime, error) {
	chats, err := h.listChats(ctx, wsID, repoID)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, len(chats))
	for i, c := range chats {
		ids[i] = c.ID
	}
	snaps, err := h.chats.ChatSnapshotsOf(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	chats = chats[:0]
	runtimes := make(map[string]dto.ChatRuntime, len(snaps))
	for _, s := range snaps {
		chat, runtime := snapshotParts(s)
		chats, runtimes[chat.ID] = append(chats, chat), runtime
	}
	return chats, runtimes, nil
}

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

func snapshotParts(s agentusecase.ChatSnapshot) (domain.Chat, dto.ChatRuntime) {
	conversation, _ := s.Conversation()
	rt := dto.ChatSnapshotRuntime(s.Live, s.Phase, s.Version,
		s.TerminalWait, s.AttachedSessionID, conversation, s.Ledger.Busy)
	rt.PendingMove = s.Ledger.PendingMove
	return s.Chat, rt
}

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
