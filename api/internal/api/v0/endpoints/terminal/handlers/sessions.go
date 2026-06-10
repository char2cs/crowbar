package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// CreateSession handles POST /v0/workspaces/:wsId/terminals.
func (h *Handlers) CreateSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

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

	libs.WriteQueryWithStatus(ctx, http.StatusCreated, gin.H{"sessionId": sid})
}

// KillSession handles DELETE /v0/terminals/:sessionId.
func (h *Handlers) KillSession(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	sid := ctx.Param("sessionId")
	if err := eng.Kill(ctx.Request.Context(), sid); err != nil {
		if errors.Is(err, engineterminal.ErrSessionNotFound) {
			libs.WriteErr(ctx, http.StatusNotFound, err.Error())
			return
		}
		libs.WriteErr(ctx, http.StatusInternalServerError, err.Error())
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
	if profileID == "" {
		return nil
	}
	p, err := h.profileStore.FindByKey(ctx, profileID)
	if err != nil {
		return nil
	}
	return p
}
