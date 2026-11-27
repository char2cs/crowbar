package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/origin"
	"github.com/char2cs/crowbar/api/internal/core/safego"
)

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 45 * time.Second
)

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 64 * 1024,
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
	defer libs.CloseLogged(ctx.Request.Context(), "terminal ws: close", conn)

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	if err := conn.SetReadDeadline(time.Now().Add(wsPongWait)); err != nil {
		slog.WarnContext(ctx.Request.Context(), "terminal ws: read deadline", "session_id", sid, "err", err)
		return
	}

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

	if err := eng.Attach(ctx.Request.Context(), sid, conn); err != nil {
		slog.WarnContext(ctx.Request.Context(), "terminal ws: attach", "session_id", sid, "err", err)
	}
}
