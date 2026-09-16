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

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateSession handles POST /v0/chats/:chatId/terminals.
//
// Per D2 the response is 201 {sessionId} synchronously (the FE needs the id
// immediately to open the raw PTY WebSocket) AND a lifecycle
// TerminalSessionDTO{status:"active"} is broadcast so the chat-scoped stream
// converges.
//
// The session is OWNED by :chatId and RUNS in the worktree that chat resolved
// to — the two arguments to eng.Create, and the whole point of the re-key.
// Sibling chats on one worktree get the same WorktreePath and disjoint
// sessions.
func (h *Handlers) CreateSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	chatID := ctx.Param("chatId")

	// Resolved once by resolveChatWorktree before this handler ran; a miss
	// means the route is mounted outside that middleware, which is a wiring
	// bug rather than anything the caller did.
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
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	h.pushSession(
		dto.TerminalSessionDTOFrom(
			sid,
			chatID,
			body.ProfileID,
			"active",
			time.Now().UTC(),
		),
	)

	libs.WriteQueryWithStatus(ctx, http.StatusCreated, gin.H{"sessionId": sid})
}

// ListSessions handles GET /v0/chats/:chatId/terminals. It returns the
// sessions OWNED BY THAT CHAT from the in-memory engine registry with their
// real lifecycle state (active|detached|suspended) (D6: terminals are
// ephemeral, no persistence). A WebSocket upgrade on the same path is routed to
// the lifecycle broadcaster by the dual-serve wrapper.
//
// A sibling chat sharing this chat's worktree is NOT listed here, and that is
// the behavioural fix the re-key delivers: keyed by workspace, this listing
// used to hand one chat every other chat's shells the moment two of them shared
// a worktree — which batch import and repo-add make the common case, not the
// exotic one.
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
// It returns 202 and broadcasts a TerminalSessionDTO{status:"ended"} frame (the
// engine's OnSessionEnded reap also emits one; the broadcaster's idempotent
// full-replace makes the duplicate harmless).
func (h *Handlers) KillSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	chatID := ctx.Param("chatId")
	sid := ctx.Param("sessionId")

	if err := eng.Kill(ctx.Request.Context(), sid); err != nil {
		if errors.Is(err, engineterminal.ErrSessionNotFound) {
			libs.WriteErr(ctx, http.StatusNotFound, err.Error())
			return
		}
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	endedAt := time.Now().UTC()
	ended := dto.TerminalSessionDTOFrom(
		sid,
		chatID,
		"",
		"ended",
		endedAt,
	)
	ended.EndedAt = &endedAt
	h.pushSession(ended)

	libs.WriteMutationOK(ctx, http.StatusAccepted, sid)
}

// pushSession forwards a lifecycle DTO to the broadcaster when one is wired.
func (h *Handlers) pushSession(
	d dto.TerminalSessionDTO,
) {
	if h.broadcast == nil {
		return
	}
	h.broadcast.Push(d)
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
	if profileID == "" {
		return nil
	}
	p, err := h.profileStore.FindByKey(ctx, profileID)
	if err != nil {
		return nil
	}
	return p
}
