// Package jsonrpc is a JSON-RPC2 client over a child process's stdio, one
// message per line (ndjson).
//
// It knows nothing about Crowbar's descriptors, canonical events, or any
// provider's method names — that translation is protocol/internal/apidriver's job.
//
// Nothing here blocks the read loop (sessions spec §2.4): responses to our own
// calls are routed inline, notifications go to a bounded mailbox (mailbox.go),
// every write has a deadline and every call a timeout.
package jsonrpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// writeTimeout bounds one message write: a peer that stops reading must not
	// hold every caller's write lock forever.
	writeTimeout = 10 * time.Second
	// defaultCallTimeout bounds a Call whose ctx carries no deadline.
	defaultCallTimeout = 60 * time.Second
)

// Stdio is a child's stdin and stdout as its parent holds them. Both exist
// before the child does, so nothing written to In can arrive too early.
type Stdio struct {
	In  Input
	Out io.ReadCloser
}

// Input is the child's stdin: a write deadline keeps a child that stopped
// reading from wedging every writer.
type Input interface {
	io.WriteCloser
	SetWriteDeadline(t time.Time) error
}

// Close ends both pipes: the child reads EOF, and our reader stops.
func (s Stdio) Close() error {
	return errors.Join(s.In.Close(), s.Out.Close())
}

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
	stdio Stdio

	wmu sync.Mutex // serialises writes: one message per line, never interleaved

	nextID  int64
	pmu     sync.Mutex
	pending map[int64]chan wireFrame

	mailbox *mailbox

	closed chan struct{}
	once   sync.Once

	callTimeout time.Duration
}

// Option configures a Conn at New.
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
	return fmt.Sprintf("jsonrpc: %s: %s (code %d)", e.Method, e.Message, e.Code)
}

// New speaks JSON-RPC over stdio until either side closes it. The child is
// ready when it answers a Call; nothing here waits for it before that.
func New(stdio Stdio, opts ...Option) *Conn {
	c := &Conn{
		stdio:       stdio,
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
	return c
}

func (c *Conn) readLoop() {
	defer c.teardown()
	lines := bufio.NewReader(c.stdio.Out)
	for {
		line, err := lines.ReadBytes('\n')
		if !c.route(bytes.TrimSpace(line)) {
			return
		}
		if err != nil {
			c.mailbox.finish()
			return
		}
	}
}

// route handles one line, reporting false once the connection must close.
func (c *Conn) route(line []byte) bool {
	var f wireFrame
	if len(line) == 0 || json.Unmarshal(line, &f) != nil {
		return true // a blank or malformed line is dropped, never fatal to the connection
	}
	if f.Method == "" && f.ID != nil {
		c.dispatchResponse(f)
		return true
	}
	if c.mailbox.push(Frame{ID: f.ID, Method: f.Method, Params: f.Params}) {
		return true
	}
	_ = c.stdio.Close() // overflowed: the consumer is wedged
	return false
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
	req, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", id, method, params})
	if err != nil {
		return nil, fmt.Errorf("jsonrpc: marshal params for %s: %w", method, err)
	}

	ch := make(chan wireFrame, 1)
	if !c.register(id, ch) {
		return nil, errors.New("jsonrpc: connection closed")
	}
	if err := c.write(req); err != nil {
		c.unregister(id)
		return nil, fmt.Errorf("jsonrpc: write %s: %w", method, err)
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
			return nil, fmt.Errorf("jsonrpc: %s: %w", method, ctx.Err())
		}
	case <-c.closed:
		select {
		case f := <-ch:
			return callResult(method, f)
		default:
			return nil, errors.New("jsonrpc: connection closed")
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
	msg, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
	if err != nil {
		return fmt.Errorf("jsonrpc: marshal params for %s: %w", method, err)
	}
	return c.write(msg)
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
		return fmt.Errorf("jsonrpc: marshal reply: %w", err)
	}
	return c.write(msg)
}

// write sends msg as one line; json.Marshal never emits a raw newline.
func (c *Conn) write(msg []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.stdio.In.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("jsonrpc: write deadline: %w", err)
	}
	if _, err := c.stdio.In.Write(append(msg, '\n')); err != nil {
		return fmt.Errorf("jsonrpc: write: %w", err)
	}
	return nil
}

// Close tears down the connection, unblocks every pending Call with an error
// and drops whatever the mailbox still held. Idempotent.
func (c *Conn) Close() error {
	c.mailbox.abandon()
	err := c.stdio.Close()
	c.teardown()
	return err
}
