package v0

import (
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gin-gonic/gin"
)

type WSFilesHandler struct {
	hub   *wshub.Hub
	store *fixtures.Store
}

func NewWSFilesHandler(hub *wshub.Hub, store *fixtures.Store) *WSFilesHandler {
	return &WSFilesHandler{hub: hub, store: store}
}

func (h *WSFilesHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	wsID := c.Query("workspaceId")
	_ = conn.WriteJSON(fixtures.FileEvent{WorkspaceID: wsID, Path: "/"})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
