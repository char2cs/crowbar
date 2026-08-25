// Package wsrpc is a WebSocket-framed JSON-RPC2 client over a unix socket.
//
// codex's `app-server --listen unix://PATH` runs a plain HTTP-Upgrade WebSocket
// handshake over the socket (tungstenite), NOT raw newline-delimited JSON like
// its `--listen stdio://` mode does — confirmed live against codex-cli 0.146.0:
// a raw writer gets `httparse error: invalid token` and the connection is closed
// with no response. This package speaks the WebSocket layer so nothing above it
// has to.
//
// It knows nothing about Crowbar's descriptors, canonical events, or codex's own
// method names — that translation is protocol/internal/apidriver's job.
package wsrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Frame is one inbound message that carries a method: either a plain
// notification (ID nil) or a server-initiated request this connection's owner
// must Reply to (ID non-nil). A bare id+result/error frame — the response to
// OUR OWN Call — is never delivered here; Call consumes it directly.
type Frame struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

type Conn struct {
	ws *websocket.Conn

	// mu guards writes: gorilla/websocket forbids concurrent writers on one
	// connection, and Call/Notify/Reply may all be invoked from different
	// goroutines.
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan wireFrame

	frames chan Frame
	closed chan struct{}
	once   sync.Once
}

type wireFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *wireError      `json:"error,omitempty"`
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Dial performs the WebSocket handshake over a unix socket at socketPath.
//
// EnableCompression is left at its zero value (false) DELIBERATELY: codex's
// server rejects a Sec-WebSocket-Extensions offer it does not recognise with
// "Missing, duplicated or incorrect header sec-websocket-extensions" and closes
// the connection — confirmed live. gorilla/websocket's default Dialer already
// offers no extensions, which is why this needs no explicit configuration.
func Dial(ctx context.Context, socketPath string) (*Conn, error) {
	dialer := websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		HandshakeTimeout: 10 * time.Second,
	}
	// The URL's host/scheme are ignored by NetDialContext; codex does not
	// validate them beyond requiring a well-formed request line.
	ws, resp, err := dialer.DialContext(ctx, "ws://unix/", http.Header{})
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return nil, fmt.Errorf("wsrpc: dial %s: %w", socketPath, err)
	}
	c := &Conn{
		ws:      ws,
		pending: make(map[int64]chan wireFrame),
		frames:  make(chan Frame, 32),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *Conn) readLoop() {
	defer func() {
		slog.Info("DIAG4: wsrpc readLoop exiting")
		c.teardown()
	}()
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			slog.Info("DIAG4: wsrpc ReadMessage error", "err", fmt.Sprint(err))
			return
		}
		slog.Info("DIAG4: wsrpc raw frame", "data", string(data))
		var f wireFrame
		if err := json.Unmarshal(data, &f); err != nil {
			continue // a malformed frame is dropped, never fatal to the connection
		}
		if f.Method == "" && f.ID != nil {
			c.dispatchResponse(f)
			continue
		}
		select {
		case c.frames <- Frame{ID: f.ID, Method: f.Method, Params: f.Params}:
		case <-c.closed:
			return
		}
	}
}

// dispatchResponse routes a reply to OUR OWN earlier Call to the goroutine
// blocked waiting for it, identified by the numeric id we minted for it.
func (c *Conn) dispatchResponse(f wireFrame) {
	var id int64
	if err := json.Unmarshal(f.ID, &id); err != nil {
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ok {
		ch <- f
	}
}

// teardown runs once, whether triggered by the read loop hitting EOF/error or
// by an explicit Close: it closes frames (so a Frames() consumer's range loop
// ends) and wakes every Call still blocked on a reply that will never arrive.
func (c *Conn) teardown() {
	c.once.Do(func() {
		close(c.closed)
		close(c.frames)
	})
}

// Frames delivers every inbound notification and server-initiated ask, in
// arrival order. Closed when the connection is closed or the server hangs up.
func (c *Conn) Frames() <-chan Frame { return c.frames }

// Call sends a JSON-RPC request and blocks for its matching response.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("wsrpc: marshal params for %s: %w", method, err)
	}
	req, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{"2.0", id, method, paramsRaw})
	if err != nil {
		return nil, fmt.Errorf("wsrpc: marshal request %s: %w", method, err)
	}

	ch := make(chan wireFrame, 1)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return nil, errors.New("wsrpc: connection closed")
	default:
	}
	c.pending[id] = ch
	writeErr := c.ws.WriteMessage(websocket.TextMessage, req)
	c.mu.Unlock()
	if writeErr != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("wsrpc: write %s: %w", method, writeErr)
	}

	select {
	case f := <-ch:
		if f.Error != nil {
			return nil, fmt.Errorf("wsrpc: %s: %s (code %d)", method, f.Error.Message, f.Error.Code)
		}
		return f.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("wsrpc: connection closed")
	}
}

// Notify sends a JSON-RPC notification (no id, no reply expected).
func (c *Conn) Notify(method string, params any) error {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("wsrpc: marshal params for %s: %w", method, err)
	}
	msg, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{"2.0", method, paramsRaw})
	if err != nil {
		return fmt.Errorf("wsrpc: marshal notify %s: %w", method, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, msg)
}

// Reply answers a server-initiated ask (a Frame with a non-nil ID) with a
// JSON-RPC response frame carrying result verbatim.
func (c *Conn) Reply(id, result json.RawMessage) error {
	msg, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", id, result})
	if err != nil {
		return fmt.Errorf("wsrpc: marshal reply: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, msg)
}

// Close tears down the connection and unblocks every pending Call with an
// error. Idempotent.
func (c *Conn) Close() error {
	err := c.ws.Close()
	c.teardown()
	return err
}
