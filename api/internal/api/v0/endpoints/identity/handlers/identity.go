package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func (h *Handlers) Get(
	c *gin.Context,
) {
	ws, err := h.resolveWorkspace(c)
	if err != nil {
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
