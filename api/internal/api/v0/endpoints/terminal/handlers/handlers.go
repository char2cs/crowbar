// Package handlers holds the gin handlers backing the terminal endpoint: the
// terminal-profile CRUD routes under /settings/terminal/profiles and the PTY
// session lifecycle (create under /workspaces/:wsId/terminals, kill under
// /terminals/:sessionId). The bidirectional PTY WebSocket lives in ws.go and is
// mounted separately because it speaks a raw frame protocol rather than the REST
// envelope.
//
// The terminal engine is guarded for nil before any session work runs; an
// absent engine yields a 503. REST responses carry the uniform
// {success,error,data} envelope; profiles serialise via their wire DTO and the
// session create returns { sessionId } under data.
package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TerminalEngine is the PTY session surface the REST handlers consume. It mirrors
// the methods of engineterminal.Engine the handlers need so the live engine
// satisfies it directly.
type TerminalEngine interface {
	Create(
		ctx context.Context,
		workspaceID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (string, error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
}

// ProfileStore is the CRUD surface over terminal profiles the handlers consume.
// It mirrors the generic store the GORM-backed profile store satisfies.
type ProfileStore interface {
	Save(
		ctx context.Context,
		item domain.TerminalProfile,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.TerminalProfile, error)
	FindAll(
		ctx context.Context,
	) ([]domain.TerminalProfile, error)
}

// WorkspaceReader resolves a workspace id to its aggregate, supplying the
// worktree path a new PTY session spawns in. It mirrors the workspace
// repository's Get method.
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Handlers serves the /v0 terminal REST routes from the terminal engine, the
// profile store, and the workspace reader. A nil engine surfaces as a 503 on the
// session routes.
type Handlers struct {
	eng      TerminalEngine
	profiles ProfileStore
	wsReader WorkspaceReader
}

// New builds the terminal Handlers from the terminal engine, the profile store,
// and the workspace reader.
func New(
	eng TerminalEngine,
	profiles ProfileStore,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		eng:      eng,
		profiles: profiles,
		wsReader: wsReader,
	}
}

func (h *Handlers) requireEngine(
	c *gin.Context,
) bool {
	if h.eng == nil {
		libs.WriteErr(
			c,
			http.StatusServiceUnavailable,
			"terminal engine not available",
		)
		return false
	}
	return true
}

func (h *Handlers) resolveProfile(
	c *gin.Context,
	profileID string,
) (*domain.TerminalProfile, bool) {
	if profileID == "" {
		return nil, true
	}
	prof, err := h.profiles.FindByKey(c.Request.Context(), profileID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return nil, false
	}
	if prof == nil {
		libs.WriteErr(c, http.StatusNotFound, "profile not found")
		return nil, false
	}
	return prof, true
}
