package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/origin"
)

// upgrader rejects cross-origin upgrades from a non-allow-listed Origin: log
// lines can carry paths and ids, so any website must not read them.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return origin.Allowed(r.Header.Get("Origin"), r.Host)
	},
}

// Logs handles GET /v0/console/logs: it upgrades to a WebSocket, replays the
// last 500 records (or those after since), sends one ready frame, then every new
// record. A client 256 records behind is closed and reconnects with since.
func (h *Handlers) Logs(
	c *gin.Context,
) {
	level, since, ok := readLogQuery(c)
	if !ok {
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	newLogSession(conn, h.logs.Stream(since, level)).run()
}

func readLogQuery(
	c *gin.Context,
) (slog.Level, uint64, bool) {
	level := slog.LevelInfo
	if raw := c.Query("level"); raw != "" && level.UnmarshalText([]byte(raw)) != nil {
		libs.WriteErr(c, http.StatusBadRequest, "level must be debug, info, warn or error")
		return level, 0, false
	}
	since, err := strconv.ParseUint(c.DefaultQuery("since", "0"), 10, 64)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "since must be an unsigned integer")
		return level, 0, false
	}
	return level, since, true
}
