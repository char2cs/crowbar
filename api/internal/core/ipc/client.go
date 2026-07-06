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

func NewClient(host string) (*Client, error) {
	sock, err := transports.SocketPath(host)
	if err != nil {
		return nil, fmt.Errorf("ipc: socket path: %w", err)
	}
	return &Client{http: &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
	}}, nil
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
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}
