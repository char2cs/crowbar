package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// createSessionBody is the optional request body for a session create: the id of
// the profile to launch the PTY with. An empty body launches the default
// profile.
type createSessionBody struct {
	ProfileID string `json:"profileId"`
}

// CreateSession handles POST /v0/workspaces/:wsId/terminals. It resolves the
// workspace to its worktree path, optionally loads the requested profile, spawns
// a PTY session, and returns its id under data: { sessionId }.
func (h *Handlers) CreateSession(
	c *gin.Context,
) {
	if !h.requireEngine(c) {
		return
	}

	wsID := c.Param("wsId")
	ws, err := h.wsReader.Get(c.Request.Context(), wsID)
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			"workspace not found",
		)
		return
	}

	var body createSessionBody
	if bindErr := c.ShouldBindJSON(&body); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		libs.WriteErr(
			c,
			http.StatusBadRequest,
			bindErr.Error(),
		)
		return
	}

	prof := h.resolveProfile(c.Request.Context(), body.ProfileID)

	sid, err := h.eng.Create(
		c.Request.Context(),
		wsID,
		ws.WorktreePath,
		prof,
	)
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	libs.WriteQueryWithStatus(
		c,
		http.StatusCreated,
		dto.TerminalSessionDTO{
			SessionID: sid,
		},
	)
}

// KillSession handles DELETE /v0/terminals/:sessionId. It terminates the PTY
// session and returns 204, or 404 when the session is unknown.
func (h *Handlers) KillSession(
	c *gin.Context,
) {
	if !h.requireEngine(c) {
		return
	}

	sid := c.Param("sessionId")
	err := h.eng.Kill(c.Request.Context(), sid)
	if errors.Is(err, engineterminal.ErrSessionNotFound) {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			err.Error(),
		)
		return
	}
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.Status(http.StatusNoContent)
}
