// Package handlers serves the browser peer socket.
package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/origin"
	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

const (
	writeTimeout = 10 * time.Second
	// readLimit admits a screenshot frame; the desktop app caps its pixel size.
	readLimit = 16 << 20
)

// upgrader rejects cross-origin upgrades: this socket can drive browser tabs.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return origin.Allowed(r.Header.Get("Origin"), r.Host)
	},
}

// Handlers serves GET /v0/browser/peer.
type Handlers struct {
	bridge *browserbridge.Bridge
}

// New builds the handlers around the bridge peers attach to.
func New(bridge *browserbridge.Bridge) *Handlers {
	return &Handlers{bridge: bridge}
}

// Peer upgrades to a WebSocket and attaches it to the bridge until it closes.
func (h *Handlers) Peer(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(readLimit)

	p := &peer{conn: conn}
	session := h.bridge.Attach(p)
	defer session.Close()
	// The peer reports its tabs in answer, so a reconnect needs no logic of its own.
	if p.Send([]byte(`{"type":"hello"}`)) != nil {
		return
	}
	for {
		kind, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.TextMessage {
			session.Receive(frame)
		}
	}
}

// peer serialises writes: the bridge may send from several agents' goroutines.
type peer struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (p *peer) Send(frame []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return p.conn.WriteMessage(websocket.TextMessage, frame)
}
