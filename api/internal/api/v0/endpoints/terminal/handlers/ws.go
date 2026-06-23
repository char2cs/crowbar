package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/origin"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/gorilla/websocket"
)

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 45 * time.Second // must be < wsPongWait
)

// terminalUpgrader rejects cross-origin WebSocket upgrades from a non-allow-listed
// Origin: the terminal socket is a live shell, so a blanket "return true" would let
// any website the user visited open a PTY through their browser. See origin.Allowed.
var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		if origin.Allowed(o, r.Host) {
			return true
		}
		slog.WarnContext(r.Context(), "terminal ws: rejected cross-origin upgrade",
			"origin", o, "host", r.Host)
		return false
	},
}

// WS handles GET .../workspaces/:wsId/terminals/:sessionId/ws.
// It verifies the session exists before upgrading, sets up ping/pong keepalive,
// then runs the read/write pumps via the terminal engine.
func (h *Handlers) WS(ctx *gin.Context) {
	eng := h.requireTerminalEngine(ctx)
	if eng == nil {
		return
	}

	sid := ctx.Param("sessionId")
	if !eng.SessionExists(ctx.Request.Context(), sid) {
		libs.WriteErr(ctx, http.StatusNotFound, "session not found")
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
		defer safego.Recover("terminal.ws.pinger")
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
