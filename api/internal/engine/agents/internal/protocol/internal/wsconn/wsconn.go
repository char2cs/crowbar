// Package wsconn is a jsonrpc.Transport over a WebSocket: one JSON-RPC message per
// text frame, with ping/pong so a dead peer fails Recv.
package wsconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/connect"
)

const (
	writeTimeout = 10 * time.Second
	pingEvery    = 20 * time.Second
	pongWait     = 3 * pingEvery / 2
)

// Conn is one live socket.
type Conn struct {
	ws        *websocket.Conn
	writeMu   sync.Mutex
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// Dial resolves an endpoint and connects. A failed handshake re-resolves once,
// because tickets are single-use; the second failure is returned.
func Dial(ctx context.Context, resolve func(context.Context) (connect.Endpoint, error)) (*Conn, error) {
	var err error
	for range 2 {
		var ep connect.Endpoint
		if ep, err = resolve(ctx); err != nil {
			return nil, fmt.Errorf("wsconn: resolve endpoint: %w", err)
		}
		var ws *websocket.Conn
		if ws, err = dialOnce(ctx, ep); err == nil {
			return newConn(ws), nil
		}
	}
	return nil, err
}

func dialOnce(ctx context.Context, ep connect.Endpoint) (*websocket.Conn, error) {
	d := websocket.Dialer{Subprotocols: ep.Protocols, HandshakeTimeout: writeTimeout}
	ws, resp, err := d.DialContext(ctx, ep.URL, ep.Header)
	if resp != nil && resp.Body != nil {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	if err != nil {
		return nil, fmt.Errorf("wsconn: dial: %w", err)
	}
	return ws, nil
}

func newConn(ws *websocket.Conn) *Conn {
	c := &Conn{ws: ws, done: make(chan struct{})}
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(pongWait)) })
	go c.ping()
	return c
}

func (c *Conn) ping() {
	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-tick.C:
			if c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)) != nil {
				return
			}
		}
	}
}

// Send writes msg as one text frame.
func (c *Conn) Send(msg []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.ws.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("wsconn: write deadline: %w", err)
	}
	if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		return fmt.Errorf("wsconn: write: %w", err)
	}
	return nil
}

// Recv returns the next text frame; a clean close is io.EOF.
func (c *Conn) Recv() ([]byte, error) {
	for {
		if err := c.ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			return nil, fmt.Errorf("wsconn: read deadline: %w", err)
		}
		kind, msg, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil, fmt.Errorf("wsconn: closed: %w", io.EOF)
			}
			return nil, fmt.Errorf("wsconn: read: %w", err)
		}
		if kind == websocket.TextMessage {
			return msg, nil
		}
	}
}

// Close sends a close frame and drops the socket; later calls return the first result.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		frame := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		err := c.ws.WriteControl(websocket.CloseMessage, frame, time.Now().Add(writeTimeout))
		if cerr := c.ws.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil && !errors.Is(err, websocket.ErrCloseSent) {
			c.closeErr = fmt.Errorf("wsconn: close: %w", err)
		}
	})
	return c.closeErr
}
