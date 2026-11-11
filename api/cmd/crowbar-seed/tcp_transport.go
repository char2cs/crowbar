package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// tcpTransport talks to a daemon bound to a TCP address, the shape `make
// dev-api` serves. It mirrors ipc.Client's contract exactly, non-2xx included,
// so callers cannot tell the two wires apart.
type tcpTransport struct {
	base string
	http *http.Client
}

func newTCPTransport(
	host string,
) *tcpTransport {
	return &tcpTransport{
		base: "http://" + strings.TrimPrefix(host, tcpScheme),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *tcpTransport) Get(
	ctx context.Context,
	path string,
) (int, []byte, error) {
	return t.do(ctx, http.MethodGet, path, nil)
}

func (t *tcpTransport) PostJSON(
	ctx context.Context,
	path string,
	body any,
) (int, []byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("tcp: marshal body for %s: %w", path, err)
	}
	return t.do(ctx, http.MethodPost, path, buf)
}

func (t *tcpTransport) PatchJSON(
	ctx context.Context,
	path string,
	body any,
) (int, []byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("tcp: marshal body for %s: %w", path, err)
	}
	return t.do(ctx, http.MethodPatch, path, buf)
}

func (t *tcpTransport) do(
	ctx context.Context,
	method string,
	path string,
	body []byte,
) (_ int, _ []byte, err error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("tcp: build %s %s: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("tcp: %s %s: %w", method, path, err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("tcp: %s %s: read: %w", method, path, err)
	}
	return resp.StatusCode, respBody, nil
}
