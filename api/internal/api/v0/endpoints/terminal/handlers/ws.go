package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
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

// NewWS builds the terminal WebSocket gin.HandlerFunc for
// GET /v0/ws/terminals/:sessionId. It verifies the session exists before
// upgrading, sets up ping/pong keepalive, then streams the PTY through the
// engine's Attach. The handler speaks the raw bidirectional PTY frame protocol
// and does not use the REST envelope. A nil engine yields a 503.
func NewWS(
	eng engineterminal.Engine,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveWS(eng, c)
	}
}

func serveWS(
	eng engineterminal.Engine,
	c *gin.Context,
) {
	if eng == nil {
		c.JSON(
			http.StatusServiceUnavailable,
			gin.H{"error": "terminal engine not available"},
		)
		return
	}

	sid := c.Param("sessionId")
	if !eng.SessionExists(c.Request.Context(), sid) {
		c.JSON(
			http.StatusNotFound,
			gin.H{"error": "session not found"},
		)
		return
	}

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer func() { _ = conn.Close() }()

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))

	// Capture the request context locally before launching the ping goroutine:
	// gin pools and reuses *gin.Context after the handler returns, so reading
	// c.Request from a goroutine that outlives the handler races the next
	// request's reset.
	reqCtx := c.Request.Context()
	go pingLoop(reqCtx, conn)

	_ = eng.Attach(reqCtx, sid, conn)
}

func pingLoop(
	ctx context.Context,
	conn *websocket.Conn,
) {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pingErr := conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(wsWriteWait),
			)
			if pingErr != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
