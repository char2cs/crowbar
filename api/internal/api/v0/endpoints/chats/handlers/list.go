package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// List handles GET /v0/workspaces/:wsId/chats, returning the workspace's chats
// as a flat ChatDTO[] list.
func (h *Handlers) List(
	c *gin.Context,
) {
	chats, err := h.chats.ListChatsByWorkspace(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ChatDTOList(chats))
}
