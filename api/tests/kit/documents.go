//go:build integration

package kit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// Local reaches a daemon handler over a unix socket, the one transport a host
// document is served on; a harness serving over TCP uses it for those routes.
type Local struct {
	client *http.Client
	dialer *websocket.Dialer
}

// ServeLocal serves handler on a fresh unix socket until the test ends.
func ServeLocal(
	t *testing.T,
	handler http.Handler,
) *Local {
	t.Helper()
	socket := kitSocketPath()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	require.NoError(t, err)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
		if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
			t.Logf("kit: remove socket: %v", err)
		}
	})
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}
	return &Local{
		client: &http.Client{Transport: &http.Transport{DialContext: dial}},
		dialer: &websocket.Dialer{NetDialContext: dial, HandshakeTimeout: 10 * time.Second},
	}
}

// ReadDocument reads the document at path and answers the path the daemon
// spelled it with, and its text.
func (l *Local) ReadDocument(
	t *testing.T,
	path string,
) (string, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://crowbar/v0/documents/content?path="+url.QueryEscape(path), nil)
	require.NoError(t, err)
	resp, err := l.client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "read %s", path)
	var env struct {
		Data struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	return env.Data.Path, env.Data.Content
}

// DialDocument holds the host document at its canonical path. It returns at
// the 101, by which the daemon has armed the document's watch.
func (l *Local) DialDocument(
	t *testing.T,
	path string,
) *WSWatcher {
	t.Helper()
	return Dial(t, l.dialer, "ws://crowbar/v0/documents/ws?path="+url.QueryEscape(path))
}
