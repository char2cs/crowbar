package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// createSessionBody is the optional request body for a session create: the id of
// the profile to launch the PTY with. An empty body launches the default
// profile.
type createSessionBody struct {
	ProfileID string `json:"profileId"`
}

// CreateSession handles POST /v0/workspaces/:wsId/terminals. It resolves the
// workspace to its worktree path, optionally loads the requested profile, spawns
// a PTY session, and returns its id under data: { sessionId }. An unknown
// profileId yields 404; a profile-store error yields 500.
func (h *Handlers) CreateSession(
	c *gin.Context,
) {
	if !h.requireEngine(c) {
		return
	}

	ws, ok := h.workspace(c)
	if !ok {
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

	prof, ok := h.resolveProfile(c, body.ProfileID)
	if !ok {
		return
	}

	sid, err := h.eng.Create(
		c.Request.Context(),
		ws.ID,
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
// session and returns the enveloped session id, or 404 when the session is
// unknown.
func (h *Handlers) KillSession(
	c *gin.Context,
) {
	if !h.requireEngine(c) {
		return
	}

	sid := c.Param("sessionId")
	err := h.eng.Kill(c.Request.Context(), sid)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}

	libs.WriteMutationOK(c, http.StatusOK, sid)
}

func (h *Handlers) workspace(
	c *gin.Context,
) (domain.Workspace, bool) {
	ws, err := h.wsReader.Get(
		c.Request.Context(),
		c.Param("wsId"),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return domain.Workspace{}, false
	}
	return ws, true
}
