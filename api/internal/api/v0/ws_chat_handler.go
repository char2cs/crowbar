package v0

import (
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gin-gonic/gin"
)

type WSChatHandler struct {
	hub *wshub.Hub
}

func NewWSChatHandler(hub *wshub.Hub) *WSChatHandler {
	return &WSChatHandler{hub: hub}
}

func (h *WSChatHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	chatID := c.Param("chatId")
	words := []string{"Hello", " from", " the", " mock", " server", "!"}
	for _, w := range words {
		_ = conn.WriteJSON(fixtures.ChatChunk{ChatID: chatID, Content: w, Done: false})
	}
	_ = conn.WriteJSON(fixtures.ChatChunk{ChatID: chatID, Done: true})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
