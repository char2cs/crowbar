package v0

import (
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gin-gonic/gin"
)

type WSTerminalHandler struct {
	hub *wshub.Hub
}

func NewWSTerminalHandler(hub *wshub.Hub) *WSTerminalHandler {
	return &WSTerminalHandler{hub: hub}
}

func (h *WSTerminalHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	sessionID := c.Param("sessionId")

	_ = conn.WriteJSON(fixtures.TerminalFrame{
		SessionID: sessionID,
		Data:      "crowbar mock terminal ready\r\n$ ",
		IsInput:   false,
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteJSON(fixtures.TerminalFrame{
			SessionID: sessionID,
			Data:      string(msg),
			IsInput:   false,
		})
	}
}
