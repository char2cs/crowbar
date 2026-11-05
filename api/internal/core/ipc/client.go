package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

// Client is a thin HTTP client that speaks to the Crowbar daemon at the address
// it bound. The HTTP host is a placeholder; DialContext always dials that address.
type Client struct {
	http *http.Client
}

// DefaultTimeout bounds one in-PTY callback (a hook, a handoff fetch): it blocks a
// vendor CLI, so a wedged daemon must not hold it open. Callers whose daemon-side work
// is git need NewClientWithTimeout instead.
const DefaultTimeout = 5 * time.Second

func NewClient(host string) (*Client, error) {
	return NewClientWithTimeout(host, DefaultTimeout)
}

// NewClientWithTimeout builds a client with a caller-chosen per-request budget, so the
// MCP relay can wait on git work without raising the hook callbacks' short timeout.
func NewClientWithTimeout(host string, timeout time.Duration) (*Client, error) {
	network, addr := "tcp", strings.TrimPrefix(host, "tcp://")
	if addr == host {
		sock, err := transports.SocketPath(host)
		if err != nil {
			return nil, fmt.Errorf("ipc: socket path: %w", err)
		}
		network, addr = "unix", sock
	}
	return &Client{http: &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}}, nil
}

// Get issues a GET request against path, returning the status and raw body. A non-2xx
// status is not an error; callers decode the envelope to tell daemon failures apart.
func (c *Client) Get(ctx context.Context, path string) (_ int, _ []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: do: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	return readResponse(resp)
}

func (c *Client) PostJSON(ctx context.Context, path string, body any) (int, []byte, error) {
	return c.doJSON(ctx, http.MethodPost, path, body)
}

// PatchJSON issues a PATCH request against path with body marshalled as JSON. Like
// PostJSON, a non-2xx status is not an error.
func (c *Client) PatchJSON(ctx context.Context, path string, body any) (int, []byte, error) {
	return c.doJSON(ctx, http.MethodPatch, path, body)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any) (_ int, _ []byte, err error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, bytes.NewReader(buf))
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: do: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	return readResponse(resp)
}

// readResponse is the status and the whole body of resp. A body cut short
// fails the call: half a reply is not an answer.
func readResponse(resp *http.Response) (int, []byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: read body: %w", err)
	}
	return resp.StatusCode, body, nil
}
