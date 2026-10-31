// Package handlers upgrades GET /v0/stream and hands the socket to the hub.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/origin"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
)

// Server serves one client socket until it ends.
type Server interface {
	Serve(ctx context.Context, conn appstream.Conn) error
}

// Handlers serves the stream route.
type Handlers struct {
	hub  Server
	keep ws.KeepAlive
}

// New builds the handlers over the hub; keep bounds each socket.
func New(hub Server, keep ws.KeepAlive) *Handlers { return &Handlers{hub: hub, keep: keep} }

// readLimit is far above a sub's size; a scope is capped well below it.
const readLimit = 1024

// The stream carries every fact the client shows, so a page on another origin must not open it.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		if origin.Allowed(o, r.Host) {
			return true
		}
		slog.WarnContext(r.Context(), "stream: rejected cross-origin upgrade", "origin", o, "host", r.Host)
		return false
	},
}

// Stream GET /v0/stream upgrades and serves the socket.
func (h *Handlers) Stream(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already answered the client
	}
	conn.SetReadLimit(readLimit)
	done := make(chan struct{})
	if err := h.keep.Arm(conn); err == nil {
		h.keep.Pings(conn, done)
		err = h.hub.Serve(c.Request.Context(), socket{Conn: conn, write: h.keep.Write})
		slog.DebugContext(c.Request.Context(), "stream: connection ended", "err", err)
	}
	close(done)
	if err := conn.Close(); err != nil {
		slog.DebugContext(c.Request.Context(), "stream: close", "err", err)
	}
}

type socket struct {
	*websocket.Conn
	write time.Duration
}

// WriteJSON writes v; a peer that cannot take it within the write bound is gone.
func (s socket) WriteJSON(v any) error {
	if err := s.SetWriteDeadline(time.Now().Add(s.write)); err != nil {
		return fmt.Errorf("stream: write deadline: %w", err)
	}
	if err := s.Conn.WriteJSON(v); err != nil {
		return fmt.Errorf("stream: write: %w", err)
	}
	return nil
}

// CloseWith sends the close frame, then closes the socket so a stuck write returns.
func (s socket) CloseWith(code int, reason string) error {
	msg := websocket.FormatCloseMessage(code, reason)
	werr := s.WriteControl(websocket.CloseMessage, msg, time.Now().Add(s.write))
	if err := s.Close(); err != nil {
		return fmt.Errorf("stream: close %d: %w", code, err)
	}
	if werr != nil {
		return fmt.Errorf("stream: close frame %d: %w", code, werr)
	}
	return nil
}
