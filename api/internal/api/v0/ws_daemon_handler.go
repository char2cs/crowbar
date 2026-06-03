package v0

import (
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gin-gonic/gin"
)

type WSDaemonHandler struct{ hub *wshub.Hub }

func NewWSDaemonHandler(hub *wshub.Hub) *WSDaemonHandler {
	return &WSDaemonHandler{hub: hub}
}

func (h *WSDaemonHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	_ = conn.WriteJSON(fixtures.DaemonStatus{Status: "running", Version: "0.1.0-mock"})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
