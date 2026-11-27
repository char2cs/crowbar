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

func (h *Handlers) CreateSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	chatID := ctx.Param("chatId")

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
			continue
		}
		sessions = append(sessions, dto.TerminalSessionDTOFrom(id, chatID, "", state, now))
	}

	libs.WriteQueryOK(ctx, sessions)
}

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

func (h *Handlers) requireTerminalEngine(ctx *gin.Context) TerminalEngine {
	if h.termEng == nil {
		libs.WriteErr(ctx, http.StatusServiceUnavailable, "terminal engine not available")
		return nil
	}
	return h.termEng
}

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
