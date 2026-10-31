package ws

import (
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// KeepAlive bounds a server socket: a ping every Ping, a peer silent for Pong
// is gone, a write that takes Write is abandoned. A half-open peer would
// otherwise hold its goroutines and queue forever.
type KeepAlive struct {
	Ping  time.Duration
	Pong  time.Duration
	Write time.Duration
}

// DefaultKeepAlive is every daemon socket's bound.
var DefaultKeepAlive = KeepAlive{Ping: pingInterval, Pong: pongTimeout, Write: writeTimeout}

func (k KeepAlive) ticker() *time.Ticker { return time.NewTicker(k.Ping) }

// Arm sets the read deadline and extends it on every pong.
func (k KeepAlive) Arm(conn *websocket.Conn) error {
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(k.Pong))
	})
	return conn.SetReadDeadline(time.Now().Add(k.Pong))
}

// Pings sends pings from aside (WriteControl may run beside a write) until done closes.
func (k KeepAlive) Pings(conn *websocket.Conn, done <-chan struct{}) {
	safego.Go("ws.keepalive", func() {
		ticker := k.ticker()
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(k.Write)) != nil {
					return
				}
			}
		}
	})
}
