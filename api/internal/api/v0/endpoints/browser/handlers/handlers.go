package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/origin"
	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

const (
	writeTimeout = 10 * time.Second
	pingPeriod   = 20 * time.Second
	pongWait     = 60 * time.Second
	readLimit    = 16 << 20
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return origin.Allowed(r.Header.Get("Origin"), r.Host)
	},
}

type Handlers struct {
	bridge *browserbridge.Bridge
	ping   time.Duration
	pong   time.Duration
}

func New(bridge *browserbridge.Bridge) *Handlers {
	return &Handlers{bridge: bridge, ping: pingPeriod, pong: pongWait}
}

func (h *Handlers) Peer(c *gin.Context) {
	if c.Request.TLS != nil {
		libs.WriteErr(c, http.StatusForbidden, "the browser peer socket is only served over the local socket")
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.DebugContext(c.Request.Context(), "browser: close the peer socket", "err", err)
		}
	}()
	conn.SetReadLimit(readLimit)
	h.keepAlive(conn)
	stopPings := make(chan struct{})
	defer close(stopPings)
	go h.pings(conn, stopPings)

	p := &peer{conn: conn}
	session := h.bridge.Attach(p)
	defer session.Close()
	if p.Send([]byte(`{"type":"hello"}`)) != nil {
		return
	}
	for {
		kind, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if conn.SetReadDeadline(time.Now().Add(h.pong)) != nil {
			return
		}
		if kind == websocket.TextMessage {
			session.Receive(frame)
		}
	}
}

func (h *Handlers) keepAlive(conn *websocket.Conn) {
	extend := func() error { return conn.SetReadDeadline(time.Now().Add(h.pong)) }
	if err := extend(); err != nil {
		slog.Debug("browser: arm the peer read deadline", "err", err)
	}
	conn.SetPongHandler(func(string) error { return extend() })
}

func (h *Handlers) pings(
	conn *websocket.Conn,
	stop <-chan struct{},
) {
	ticker := time.NewTicker(h.ping)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)) != nil {
				return
			}
		}
	}
}

type peer struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (p *peer) Send(frame []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("browser: write deadline: %w", err)
	}
	if err := p.conn.WriteMessage(websocket.TextMessage, frame); err != nil {
		return fmt.Errorf("browser: send: %w", err)
	}
	return nil
}
