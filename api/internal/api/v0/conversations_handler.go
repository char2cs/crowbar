package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type ConversationsHandler struct{ store *fixtures.Store }

func NewConversationsHandler(store *fixtures.Store) *ConversationsHandler {
	return &ConversationsHandler{store: store}
}

func (h *ConversationsHandler) Get(c *gin.Context) {
	wsID := c.Param("wsId")
	msgs, ok := h.store.Conversations[wsID]
	if !ok {
		msgs = h.store.Conversations["default"]
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}
