package handlers

import (
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

type KeepAlive struct {
	Ping  time.Duration
	Pong  time.Duration
	Write time.Duration
}

var DefaultKeepAlive = KeepAlive{Ping: 30 * time.Second, Pong: 60 * time.Second, Write: 10 * time.Second}

func (k KeepAlive) ticker() *time.Ticker { return time.NewTicker(k.Ping) }

func (k KeepAlive) Arm(conn *websocket.Conn) error {
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(k.Pong))
	})
	return conn.SetReadDeadline(time.Now().Add(k.Pong))
}

func (k KeepAlive) Pings(conn *websocket.Conn, done <-chan struct{}) {
	safego.Go("stream.keepalive", func() {
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
