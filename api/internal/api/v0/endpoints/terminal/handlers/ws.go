package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 45 * time.Second // must be < wsPongWait
)

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(_ *http.Request) bool { return true },
}

// WS handles GET /v0/ws/terminals/:sessionId.
// It verifies the session exists before upgrading, sets up ping/pong keepalive,
// then runs the read/write pumps via the terminal engine.
func (h *Handlers) WS(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	sid := ctx.Param("sessionId")
	if !eng.SessionExists(ctx.Request.Context(), sid) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	conn, err := terminalUpgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer func() { _ = conn.Close() }()

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))

	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if pingErr := conn.WriteControl(
					websocket.PingMessage,
					nil,
					time.Now().Add(wsWriteWait),
				); pingErr != nil {
					return
				}
			case <-ctx.Request.Context().Done():
				return
			}
		}
	}()

	_ = eng.Attach(ctx.Request.Context(), sid, conn)
}
