package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// Get handles GET /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/identity
// and GET /v0/chats/:chatId/identity.
// It returns the current human's GitHub/git identity as a best-effort response:
// the request always succeeds with 200, even when gh is absent or the workspace
// is unknown (all fields may be empty in that case).
func (h *Handlers) Get(
	c *gin.Context,
) {
	ws, err := h.resolveWorkspace(c)
	if err != nil {
		// Workspace not found — return an empty identity rather than erroring.
		libs.WriteQueryOK(c, gin.H{
			"login":       "",
			"displayName": "",
			"avatarUrl":   "",
		})
		return
	}

	id := h.identity.CurrentIdentity(c.Request.Context(), ws.WorktreePath)
	c.JSON(http.StatusOK, libs.Envelope{
		Success: true,
		Data:    id,
	})
}
