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

	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// CreateSession handles POST
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals.
//
// Per D2 the response is 201 {sessionId} synchronously (the FE needs the id
// immediately to open the raw PTY WebSocket) AND a lifecycle
// TerminalSessionDTO{status:"active"} is broadcast so the workspace-scoped
// stream converges.
func (h *Handlers) CreateSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	projectID := ctx.Param("projectId")
	repoID := ctx.Param("repoId")
	wsID := ctx.Param("wsId")

	ws, err := h.wsReader.Get(ctx.Request.Context(), wsID)
	if err != nil {
		libs.WriteErr(ctx, http.StatusNotFound, "workspace not found")
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
		wsID,
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
			wsID,
			projectID,
			repoID,
			body.ProfileID,
			"active",
			time.Now().UTC(),
		),
	)

	libs.WriteQueryWithStatus(ctx, http.StatusCreated, gin.H{"sessionId": sid})
}

// ListSessions handles GET
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals. It returns
// the sessions for the workspace from the in-memory engine registry with their
// real lifecycle state (active|detached|suspended) (D6: terminals are ephemeral,
// no persistence). A WebSocket upgrade on the same path is routed to the
// lifecycle broadcaster by the dual-serve wrapper.
func (h *Handlers) ListSessions(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	projectID := ctx.Param("projectId")
	repoID := ctx.Param("repoId")
	wsID := ctx.Param("wsId")

	ids := eng.ListSessionsForWorkspace(wsID)
	now := time.Now().UTC()
	sessions := make([]dto.TerminalSessionDTO, 0, len(ids))
	for _, id := range ids {
		state, ok := eng.StateOf(id)
		if !ok {
			continue // session vanished between List and StateOf
		}
		sessions = append(sessions, dto.TerminalSessionDTOFrom(id, wsID, projectID, repoID, "", state, now))
	}

	libs.WriteQueryOK(ctx, sessions)
}

// KillSession handles DELETE
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId.
// It returns 202 and broadcasts a TerminalSessionDTO{status:"ended"} frame (the
// engine's OnSessionEnded reap also emits one; the broadcaster's idempotent
// full-replace makes the duplicate harmless).
func (h *Handlers) KillSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	projectID := ctx.Param("projectId")
	repoID := ctx.Param("repoId")
	wsID := ctx.Param("wsId")
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
		wsID,
		projectID,
		repoID,
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
