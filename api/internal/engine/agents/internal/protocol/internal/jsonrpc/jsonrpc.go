package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	writeTimeout       = 10 * time.Second
	defaultCallTimeout = 60 * time.Second
)

type Stdio struct {
	In  Input
	Out io.ReadCloser
}

type Input interface {
	io.WriteCloser
	SetWriteDeadline(t time.Time) error
}

func (s Stdio) Close() error {
	return errors.Join(s.In.Close(), s.Out.Close())
}

type Frame struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

type Conn struct {
	t Transport

	wmu sync.Mutex

	nextID  int64
	pmu     sync.Mutex
	pending map[int64]chan wireFrame

	mailbox *mailbox

	closed chan struct{}
	once   sync.Once

	callTimeout time.Duration
}

type Option func(*Conn)

type wireFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *wireError      `json:"error,omitempty"`
}

type wireError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type CallError struct {
	Method  string
	Code    int
	Message string
	Detail  string
}

func (e *CallError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("jsonrpc: %s: %s (code %d)", e.Method, e.Message, e.Code)
	}
	return fmt.Sprintf("jsonrpc: %s: %s (code %d): %s", e.Method, e.Message, e.Code, e.Detail)
}

const maxDetail = 200

func errorDetail(raw json.RawMessage) string {
	var data struct {
		Reason string   `json:"reason"`
		Fields []string `json:"fields"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	parts := []string{}
	if data.Reason != "" {
		parts = append(parts, data.Reason)
	}
	if len(data.Fields) > 0 {
		parts = append(parts, "fields: "+strings.Join(data.Fields, ", "))
	}
	return truncate(strings.Join(parts, "; "), maxDetail)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func New(stdio Stdio, opts ...Option) *Conn {
	return NewTransport(&stdioTransport{Stdio: stdio, lines: bufio.NewReader(stdio.Out)}, opts...)
}

func NewTransport(t Transport, opts ...Option) *Conn {
	c := &Conn{
		t:           t,
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
	for {
		msg, err := c.t.Recv()
		if err != nil {
			c.mailbox.finish()
			return
		}
		if !c.route(msg) {
			return
		}
	}
}

func (c *Conn) route(msg []byte) bool {
	var f wireFrame
	if json.Unmarshal(msg, &f) != nil {
		return true
	}
	if f.Method == "" && f.ID != nil {
		c.dispatchResponse(f)
		return true
	}
	if c.mailbox.push(Frame{ID: f.ID, Method: f.Method, Params: f.Params}) {
		return true
	}
	if err := c.t.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("jsonrpc: close after mailbox overflow", "err", err)
	}
	return false
}

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
		ch <- f
	}
}

func (c *Conn) teardown() {
	c.once.Do(func() { close(c.closed) })
}

func (c *Conn) Frames() <-chan Frame { return c.mailbox.out }

func (c *Conn) Overflowed() bool { return c.mailbox.overflowed() }

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

func callResult(method string, f wireFrame) (json.RawMessage, error) {
	if f.Error != nil {
		return nil, &CallError{Method: method, Code: f.Error.Code, Message: f.Error.Message, Detail: errorDetail(f.Error.Data)}
	}
	return f.Result, nil
}

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

func (c *Conn) ReplyError(id json.RawMessage, code int, message string) error {
	msg, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   wireError       `json:"error"`
	}{"2.0", id, wireError{Code: code, Message: message}})
	if err != nil {
		return fmt.Errorf("jsonrpc: marshal error reply: %w", err)
	}
	return c.write(msg)
}

func (c *Conn) write(msg []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.t.Send(msg)
}

func (c *Conn) Close() error {
	c.mailbox.abandon()
	err := c.t.Close()
	c.teardown()
	return err
}
