package v0

import (
	"net/http"

	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

type WSWorkspacesHandler struct {
	hub   *wshub.Hub
	store *fixtures.Store
}

func NewWSWorkspacesHandler(hub *wshub.Hub, store *fixtures.Store) *WSWorkspacesHandler {
	return &WSWorkspacesHandler{hub: hub, store: store}
}

func (h *WSWorkspacesHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	_ = conn.WriteJSON(fixtures.WorkspaceEvent{WorkspaceID: "", Action: "snapshot"})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
