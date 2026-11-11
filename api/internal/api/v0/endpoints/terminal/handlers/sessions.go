package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateSession handles POST /v0/chats/:chatId/terminals, answering 201 {sessionId} synchronously
// so the client can open the PTY WebSocket. The session is owned by :chatId and runs in its
// worktree; sibling chats on one worktree get disjoint sessions.
func (h *Handlers) CreateSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	chatID := ctx.Param("chatId")

	// A miss means the route is mounted outside resolveChatWorktree: a wiring bug.
	ws, ok := reqscope.Workspace(ctx)
	if !ok {
		libs.WriteErr(ctx, http.StatusInternalServerError, "chat worktree not resolved")
		return
	}

	var body struct {
		ProfileID string `json:"profileId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	prof := h.resolveProfile(ctx.Request.Context(), body.ProfileID)

	sid, err := eng.Create(
		ctx.Request.Context(),
		chatID,
		ws.WorktreePath,
		prof,
	)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryWithStatus(ctx, http.StatusCreated, gin.H{"sessionId": sid})
}

// ListSessions handles GET /v0/chats/:chatId/terminals: the sessions owned by this chat (not its
// worktree siblings) from the in-memory registry, with lifecycle state. Terminals are not persisted.
func (h *Handlers) ListSessions(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	chatID := ctx.Param("chatId")

	ids := eng.ListSessionsForChat(chatID)
	now := time.Now().UTC()
	sessions := make([]dto.TerminalSessionDTO, 0, len(ids))
	for _, id := range ids {
		state, ok := eng.StateOf(id)
		if !ok {
			continue // session vanished between List and StateOf
		}
		sessions = append(sessions, dto.TerminalSessionDTOFrom(id, chatID, "", state, now))
	}

	libs.WriteQueryOK(ctx, sessions)
}

// KillSession handles DELETE /v0/chats/:chatId/terminals/:sessionId.
// It answers 200 with the killed session's id.
func (h *Handlers) KillSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	sid := ctx.Param("sessionId")

	if err := eng.Kill(ctx.Request.Context(), sid); err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, sid)
}

// requireTerminalEngine returns the engine or writes a 503 and returns nil.
func (h *Handlers) requireTerminalEngine(ctx *gin.Context) TerminalEngine {
	if h.termEng == nil {
		libs.WriteErr(ctx, http.StatusServiceUnavailable, "terminal engine not available")
		return nil
	}
	return h.termEng
}

// resolveProfile fetches a profile by ID or returns nil if empty/missing.
func (h *Handlers) resolveProfile(ctx context.Context, profileID string) *domain.TerminalProfile {
	if profileID == "" || h.profileStore == nil {
		return nil
	}
	p, err := h.profileStore.FindByKey(ctx, profileID)
	if err != nil {
		return nil
	}
	return p
}
