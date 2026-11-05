package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// Get handles GET /v0/chats/:chatId/identity, returning the human's GitHub/git
// identity best-effort: always 200, with empty fields when gh is absent or the
// workspace is unknown.
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
