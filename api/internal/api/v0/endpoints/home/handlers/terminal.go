package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/origin"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

var homeTerminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		if origin.Allowed(o, r.Host) {
			return true
		}
		slog.WarnContext(r.Context(), "home terminal ws: rejected cross-origin upgrade",
			"origin", o, "host", r.Host)
		return false
	},
}

// ListTerminals handles GET /v0/projects/:projectId/home/terminals.
func (h *Handlers) ListTerminals(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	sessions := h.termEng.ListSessionsForWorkspace(ws.ID)
	if sessions == nil {
		sessions = []string{}
	}
	libs.WriteQueryOK(c, sessions)
}

// CreateTerminal handles POST /v0/projects/:projectId/home/terminals.
func (h *Handlers) CreateTerminal(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	sessionID, err := h.termEng.Create(c.Request.Context(), ws.ID, ws.WorktreePath, nil)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusCreated, gin.H{"sessionId": sessionID})
}

// TerminalWS handles GET /v0/projects/:projectId/home/terminals/:sessionId/ws.
func (h *Handlers) TerminalWS(c *gin.Context) {
	sid := c.Param("sessionId")
	if !h.termEng.SessionExists(c.Request.Context(), sid) {
		libs.WriteErr(c, http.StatusNotFound, "session not found")
		return
	}

	conn, err := homeTerminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer func() { _ = conn.Close() }()

	const wsPongWait = 60 * time.Second
	const wsPingPeriod = 45 * time.Second
	const wsWriteWait = 10 * time.Second

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))

	go func() {
		defer safego.Recover("home.terminal.ws.pinger")
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if pingErr := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteWait)); pingErr != nil {
					return
				}
			case <-c.Request.Context().Done():
				return
			}
		}
	}()

	_ = h.termEng.Attach(c.Request.Context(), sid, conn)
}

// KillTerminal handles DELETE /v0/projects/:projectId/home/terminals/:sessionId.
func (h *Handlers) KillTerminal(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	// Verify the session belongs to this home workspace before killing it.
	sessions := h.termEng.ListSessionsForWorkspace(ws.ID)
	found := false
	for _, s := range sessions {
		if s == sessionID {
			found = true
			break
		}
	}
	if !found {
		libs.WriteErr(c, http.StatusNotFound, "session not found")
		return
	}
	if err := h.termEng.Kill(c.Request.Context(), sessionID); err != nil {
		if errors.Is(err, engineterminal.ErrSessionNotFound) {
			libs.WriteErr(c, http.StatusNotFound, err.Error())
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	libs.WriteMutationOK(c, http.StatusAccepted, sessionID)
}
