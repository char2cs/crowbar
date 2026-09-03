package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

// Client is a thin HTTP client that speaks to the Crowbar daemon over its unix
// socket. The HTTP host is a placeholder; DialContext always dials the socket.
type Client struct {
	http *http.Client
}

// DefaultTimeout bounds one in-PTY callback into the daemon: a hook posting a
// turn, a handoff fetch. Those are small reads and writes against local state,
// and the process making them is blocking a vendor CLI, so the budget is short
// on purpose — a wedged daemon must not hold a hook open.
//
// It is NOT the budget for every caller. A request whose daemon-side work is a
// git operation needs a budget consistent with the daemon's own git ceiling, or
// the client gives up on work the daemon was still legitimately doing and
// reports a transport failure for it. See NewClientWithTimeout.
const DefaultTimeout = 5 * time.Second

func NewClient(host string) (*Client, error) {
	return NewClientWithTimeout(host, DefaultTimeout)
}

// NewClientWithTimeout builds a client whose per-request budget the CALLER
// chooses, for callers whose daemon-side work is not a small local read.
//
// It exists so that raising one caller's budget cannot silently raise
// everyone's: the client is shared by the hook and handoff callbacks, where a
// short timeout is the correct behaviour, and by the MCP relay, where it is not.
func NewClientWithTimeout(host string, timeout time.Duration) (*Client, error) {
	sock, err := transports.SocketPath(host)
	if err != nil {
		return nil, fmt.Errorf("ipc: socket path: %w", err)
	}
	return &Client{http: &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
	}}, nil
}

// Get issues a GET request against path over the daemon's unix socket,
// returning the response status and raw body. It never returns a non-nil
// error on a non-2xx status; callers decode the {success,error,data} envelope
// themselves to distinguish daemon-reported failures from transport failures.
func (c *Client) Get(ctx context.Context, path string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func (c *Client) PostJSON(ctx context.Context, path string, body any) (int, []byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(buf))
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}
