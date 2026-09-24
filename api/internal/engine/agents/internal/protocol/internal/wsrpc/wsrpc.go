// Package wsrpc is a WebSocket-framed JSON-RPC2 client over a unix socket.
//
// codex's `app-server --listen unix://PATH` speaks an HTTP-Upgrade WebSocket
// over the socket, not raw newline-delimited JSON (verified on 0.146.0).
//
// It knows nothing about Crowbar's descriptors, canonical events, or codex's own
// method names — that translation is protocol/internal/apidriver's job.
//
// Nothing here blocks the read loop (sessions spec §2.4): responses to our own
// calls are routed inline, notifications go to a bounded mailbox (mailbox.go),
// every write has a deadline and every call a timeout.
package wsrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeTimeout bounds one frame write: a peer that stops reading must not
	// hold every caller's write lock forever.
	writeTimeout = 10 * time.Second
	// defaultCallTimeout bounds a Call whose ctx carries no deadline.
	defaultCallTimeout = 60 * time.Second
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

// Conn is one client connection.
type Conn struct {
	ws *websocket.Conn

	wmu sync.Mutex // serialises writes: gorilla/websocket allows one writer

	nextID  int64
	pmu     sync.Mutex
	pending map[int64]chan wireFrame

	mailbox *mailbox

	closed chan struct{}
	once   sync.Once

	callTimeout time.Duration
}

// Option configures a Conn at Dial.
type Option func(*Conn)

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

// CallError is the error response the server returned to one of OUR OWN Calls,
// kept structured: Code is the only machine-readable part of a JSON-RPC
// failure ("session gone" vs "payload malformed").
type CallError struct {
	Method  string
	Code    int
	Message string
}

func (e *CallError) Error() string {
	return fmt.Sprintf("wsrpc: %s: %s (code %d)", e.Method, e.Message, e.Code)
}

// Dial performs the WebSocket handshake over a unix socket at socketPath. No
// compression extension is offered: codex rejects one it does not recognise.
func Dial(ctx context.Context, socketPath string, opts ...Option) (*Conn, error) {
	dialer := websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		HandshakeTimeout: 10 * time.Second,
	}
	// The URL's host/scheme are ignored by NetDialContext.
	ws, resp, err := dialer.DialContext(ctx, "ws://unix/", http.Header{})
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return nil, fmt.Errorf("wsrpc: dial %s: %w", socketPath, err)
	}
	c := &Conn{
		ws:          ws,
		pending:     make(map[int64]chan wireFrame),
		mailbox:     newMailbox(),
		closed:      make(chan struct{}),
		callTimeout: defaultCallTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	go c.readLoop()
	go c.mailbox.deliver()
	return c, nil
}

func (c *Conn) readLoop() {
	defer c.teardown()
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.mailbox.finish()
			return
		}
		var f wireFrame
		if err := json.Unmarshal(data, &f); err != nil {
			continue // a malformed frame is dropped, never fatal to the connection
		}
		if f.Method == "" && f.ID != nil {
			c.dispatchResponse(f)
			continue
		}
		if !c.mailbox.push(Frame{ID: f.ID, Method: f.Method, Params: f.Params}) {
			_ = c.ws.Close() // overflowed: the consumer is wedged
			return
		}
	}
}

// dispatchResponse routes a reply to OUR OWN earlier Call to the goroutine
// waiting for it, identified by the numeric id we minted for it.
func (c *Conn) dispatchResponse(f wireFrame) {
	var id int64
	if err := json.Unmarshal(f.ID, &id); err != nil {
		return
	}
	c.pmu.Lock()
	ch, ok := c.pending[id]
	delete(c.pending, id)
	c.pmu.Unlock()
	if ok {
		ch <- f // cap 1, and each id is answered once: never blocks
	}
}

// teardown runs once, whether the read loop ended or Close was called: it
// wakes every Call still waiting on a reply that will never arrive.
func (c *Conn) teardown() {
	c.once.Do(func() { close(c.closed) })
}

// Frames delivers every inbound notification and server-initiated ask, in
// arrival order. Closed after the connection closes and what it had already
// read is delivered.
func (c *Conn) Frames() <-chan Frame { return c.mailbox.out }


// Overflowed reports whether the connection was closed because its consumer
// fell further behind than the mailbox allows.
func (c *Conn) Overflowed() bool { return c.mailbox.overflowed() }

// Call sends a JSON-RPC request and blocks for its matching response, bounded
// by ctx or, when ctx has no deadline, by the connection's call timeout.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.callTimeout)
		defer cancel()
	}
	id := atomic.AddInt64(&c.nextID, 1)
	req, err := encode(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", id, method, params})
	if err != nil {
		return nil, fmt.Errorf("wsrpc: marshal params for %s: %w", method, err)
	}

	ch := make(chan wireFrame, 1)
	if !c.register(id, ch) {
		return nil, errors.New("wsrpc: connection closed")
	}
	if err := c.write(req); err != nil {
		c.unregister(id)
		return nil, fmt.Errorf("wsrpc: write %s: %w", method, err)
	}
	return c.await(ctx, method, id, ch)
}

func (c *Conn) register(id int64, ch chan wireFrame) bool {
	c.pmu.Lock()
	defer c.pmu.Unlock()
	select {
	case <-c.closed:
		return false
	default:
	}
	c.pending[id] = ch
	return true
}

func (c *Conn) unregister(id int64) {
	c.pmu.Lock()
	defer c.pmu.Unlock()
	delete(c.pending, id)
}

// await prefers an answer that already arrived over a ctx or close signal that
// fired around the same time (ch is buffered, so it can hold one).
func (c *Conn) await(ctx context.Context, method string, id int64, ch chan wireFrame) (json.RawMessage, error) {
	select {
	case f := <-ch:
		return callResult(method, f)
	case <-ctx.Done():
		c.unregister(id)
		select {
		case f := <-ch:
			return callResult(method, f)
		default:
			return nil, fmt.Errorf("wsrpc: %s: %w", method, ctx.Err())
		}
	case <-c.closed:
		select {
		case f := <-ch:
			return callResult(method, f)
		default:
			return nil, errors.New("wsrpc: connection closed")
		}
	}
}

// callResult converts a dispatched response frame into Call's return values.
func callResult(method string, f wireFrame) (json.RawMessage, error) {
	if f.Error != nil {
		return nil, &CallError{Method: method, Code: f.Error.Code, Message: f.Error.Message}
	}
	return f.Result, nil
}

// Notify sends a JSON-RPC notification (no id, no reply expected).
func (c *Conn) Notify(method string, params any) error {
	msg, err := encode(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
	if err != nil {
		return fmt.Errorf("wsrpc: marshal params for %s: %w", method, err)
	}
	return c.write(msg)
}

// Reply answers a server-initiated ask (a Frame with a non-nil ID) with a
// JSON-RPC response frame carrying result verbatim.
func (c *Conn) Reply(id, result json.RawMessage) error {
	msg, err := encode(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", id, result})
	if err != nil {
		return fmt.Errorf("wsrpc: marshal reply: %w", err)
	}
	return c.write(msg)
}

func (c *Conn) write(msg []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.ws.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return c.ws.WriteMessage(websocket.TextMessage, msg)
}

func encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Close tears down the connection, unblocks every pending Call with an error
// and drops whatever the mailbox still held. Idempotent.
func (c *Conn) Close() error {
	c.mailbox.abandon()
	err := c.ws.Close()
	c.teardown()
	return err
}
