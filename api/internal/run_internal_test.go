package internal

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/hookserver"
)

// TestRun_ServeError_SurfacesNotHangs proves H16: when Serve fails (the listener
// is invalid / accept fails permanently), Run must return the error so the
// process exits non-zero and its supervisor restarts the daemon — rather than
// silently blocking forever on <-ctx.Done() while the desktop spins reconnecting
// to a socket that never accepts.
func TestRun_ServeError_SurfacesNotHangs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, ln.Close()) // closed listener -> Serve returns immediately

	c := &Container{
		server:    &http.Server{Handler: http.NewServeMux()},
		listeners: []net.Listener{ln},
		hooks:     newHooks(t),
	}

	// Block on Run's own return — the real signal. A 2-second deadline here would
	// be a guess about how fast a loaded machine schedules a goroutine; if Run
	// ever regresses to hanging on <-ctx.Done(), this receive never completes and
	// `go test -timeout` is the backstop that says so.
	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	require.Error(t, <-done, "Run must surface the Serve failure instead of swallowing it")
}

// TestRun_ServesEveryListener proves a network-exposed daemon answers on its
// local unix socket AND its TCP bind at once, and that a failure on either
// listener still surfaces from Run.
func TestRun_ServesEveryListener(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "cb") // short: macOS caps sun_path at 104 bytes
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	unixLn, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unixLn.Close() })
	tcpLn, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	c := &Container{
		server: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "ok")
		})},
		listeners: []net.Listener{unixLn, tcpLn},
		hooks:     newHooks(t),
	}
	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	overUnix := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	t.Cleanup(overUnix.CloseIdleConnections)
	overTCP := &http.Client{Transport: &http.Transport{}}
	t.Cleanup(overTCP.CloseIdleConnections)

	assert.Equal(t, "ok", get(t, overUnix, "http://daemon/"), "the local unix socket must be served")
	assert.Equal(t, "ok", get(t, overTCP, "http://"+tcpLn.Addr().String()+"/"), "the TCP bind must be served")

	require.NoError(t, tcpLn.Close())
	require.Error(t, <-done, "a failure on the second listener must surface from Run too")
}

// TestRun_ClosesAnIdleKeepAliveConnection proves the server drops a
// connection left idle after a request, so an unauthenticated peer cannot hold
// one open forever — while a hijacked connection (a WebSocket) outlives that
// same idle period. The read deadline only bounds a failure.
func TestRun_ClosesAnIdleKeepAliveConnection(t *testing.T) {
	tcpLn, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	c := &Container{
		server:    newHTTPServer(http.HandlerFunc(echoOrOK), 50*time.Millisecond),
		listeners: []net.Listener{tcpLn},
		hooks:     newHooks(t),
	}
	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()
	t.Cleanup(func() {
		_ = tcpLn.Close()
		<-done
		_ = c.server.Close()
	})

	hijacked, hijackedReader := dialAndRequest(t, tcpLn.Addr().String(), "/hijack")
	line, err := hijackedReader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "hijacked\n", line)

	idle, idleReader := dialAndRequest(t, tcpLn.Addr().String(), "/")
	resp, err := http.ReadResponse(idleReader, nil)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.False(t, resp.Close, "the response must keep the connection alive")
	require.NoError(t, idle.SetReadDeadline(time.Now().Add(10*time.Second)))
	_, err = idleReader.ReadByte()
	require.ErrorIs(t, err, io.EOF, "the server must close a keep-alive connection left idle")

	_, err = io.WriteString(hijacked, "still here\n")
	require.NoError(t, err)
	line, err = hijackedReader.ReadString('\n')
	require.NoError(t, err, "a hijacked connection must outlive the idle timeout")
	assert.Equal(t, "still here\n", line)
}

func newHooks(
	t *testing.T,
) *hookserver.Server {
	t.Helper()
	hooks, err := hookserver.Listen(context.Background(), http.NewServeMux())
	require.NoError(t, err)
	t.Cleanup(func() { _ = hooks.Close() })
	return hooks
}

// echoOrOK answers "ok", or on /hijack takes the connection over and echoes
// each line back, as a WebSocket handler holds its connection.
func echoOrOK(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.URL.Path != "/hijack" {
		_, _ = io.WriteString(w, "ok")
		return
	}
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_, _ = rw.WriteString("hijacked\n")
	for rw.Flush() == nil {
		line, err := rw.ReadString('\n')
		if err != nil {
			return
		}
		_, _ = rw.WriteString(line)
	}
}

func dialAndRequest(
	t *testing.T,
	addr, path string,
) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = io.WriteString(conn, "GET "+path+" HTTP/1.1\r\nHost: daemon\r\n\r\n")
	require.NoError(t, err)
	return conn, bufio.NewReader(conn)
}

func get(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
