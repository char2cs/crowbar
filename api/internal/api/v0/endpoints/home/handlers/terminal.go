package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// ListTerminals handles GET /v0/projects/:projectId/home/terminals.
func (h *Handlers) ListTerminals(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	sessions := h.termEng.ListSessionsForWorkspace(ws.ID)
	if sessions == nil {
		sessions = []string{}
	}
	libs.WriteQueryOK(c, sessions)
}

// CreateTerminal handles POST /v0/projects/:projectId/home/terminals.
func (h *Handlers) CreateTerminal(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	sessionID, err := h.termEng.Create(c.Request.Context(), ws.ID, ws.WorktreePath, nil)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusCreated, gin.H{"sessionId": sessionID})
}

// KillTerminal handles DELETE /v0/projects/:projectId/home/terminals/:sessionId.
func (h *Handlers) KillTerminal(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	// Verify the session belongs to this home workspace before killing it.
	sessions := h.termEng.ListSessionsForWorkspace(ws.ID)
	found := false
	for _, s := range sessions {
		if s == sessionID {
			found = true
			break
		}
	}
	if !found {
		libs.WriteErr(c, http.StatusNotFound, "session not found")
		return
	}
	if err := h.termEng.Kill(c.Request.Context(), sessionID); err != nil {
		if errors.Is(err, engineterminal.ErrSessionNotFound) {
			libs.WriteErr(c, http.StatusNotFound, err.Error())
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteMutationOK(c, http.StatusAccepted, sessionID)
}
