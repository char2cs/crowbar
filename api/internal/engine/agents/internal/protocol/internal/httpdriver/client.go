// Package httpdriver drives a provider over HTTP: named request/response calls plus one
// Server-Sent-Events stream for its events. It is a sibling of jsonrpc, adapted to the
// apidriver the same way: Call for steps, Frames for inbound events, Reply for a
// provider's questions, Close to end it all.
package httpdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxResponse    = 8 << 20
	frameBuffer    = 64
)

// Frame is one stream event: Method is the SSE event name and Params its JSON data.
// Err is set instead when the event could not be understood or the stream gave up.
type Frame struct {
	Method string
	Params json.RawMessage
	Err    error
}

// Config describes one provider. Bearer is already resolved by the caller and is
// only ever sent as a header. Backoff defaults to 500ms per attempt; MaxReconnects
// bounds consecutive failed reconnects (default 5).
type Config struct {
	BaseURL       string
	Requests      map[string]Request
	StreamPath    string // may hold {vars} captured by a request; opens once all are known
	Bearer        string
	Timeout       time.Duration
	MaxReconnects int
	Backoff       func(attempt int) time.Duration
	HTTP          *http.Client
}

// Client implements the surface the driver consumes:
//
//	Call(ctx, name, params) (json.RawMessage, error)
//	Frames() <-chan Frame
//	Reply(ctx, requestName, askID, payload) error
//	Close() error
type Client struct {
	cfg    Config
	http   *http.Client
	ctx    context.Context
	cancel context.CancelFunc
	frames chan Frame
	wg     sync.WaitGroup
	end    sync.Once

	mu       sync.Mutex
	captured map[string]string
	started  bool
	closed   bool
}

// New returns a Client; the stream opens after the first call that makes
// StreamPath resolvable.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxReconnects <= 0 {
		cfg.MaxReconnects = 5
	}
	if cfg.Backoff == nil {
		cfg.Backoff = func(attempt int) time.Duration { return time.Duration(attempt) * 500 * time.Millisecond }
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		cfg: cfg, http: cfg.HTTP, ctx: ctx, cancel: cancel,
		frames: make(chan Frame, frameBuffer), captured: map[string]string{},
	}
}

// Call runs the named request and returns its response body.
func (c *Client) Call(ctx context.Context, name string, params json.RawMessage) (json.RawMessage, error) {
	values, err := paramValues(params)
	if err != nil {
		return nil, fmt.Errorf("httpdriver: %s: %w", name, err)
	}
	return c.call(ctx, name, values)
}

// Reply answers a provider question: the payload's members and ask_id fill the request.
func (c *Client) Reply(ctx context.Context, requestName, askID string, payload json.RawMessage) error {
	values, err := paramValues(payload)
	if err != nil {
		return fmt.Errorf("httpdriver: %s: %w", requestName, err)
	}
	values["ask_id"] = askID
	_, err = c.call(ctx, requestName, values)
	return err
}

func (c *Client) call(ctx context.Context, name string, values map[string]string) (json.RawMessage, error) {
	if c.isClosed() {
		return nil, errors.New("httpdriver: closed")
	}
	out, err := c.do(ctx, name, values)
	if err != nil {
		return nil, err
	}
	c.openStream()
	return out, nil
}

// Frames delivers stream events; it closes when the stream gives up or on Close.
func (c *Client) Frames() <-chan Frame { return c.frames }

// Close cancels in-flight calls and the stream, waits for them, and closes Frames.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cancel()
	c.wg.Wait()
	c.end.Do(func() { close(c.frames) })
	return nil
}

func (c *Client) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// snapshot layers params over captures so a call can use either.
func (c *Client) snapshot(params map[string]string) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.captured)+len(params))
	for k, v := range c.captured {
		out[k] = v
	}
	for k, v := range params {
		out[k] = v
	}
	return out
}

func (c *Client) authorize(r *http.Request) {
	if c.cfg.Bearer != "" {
		r.Header.Set("Authorization", "Bearer "+c.cfg.Bearer)
	}
}
