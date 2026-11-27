package internal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestRun_ServeError_SurfacesNotHangs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	c := serving(t, &http.Server{Handler: http.NewServeMux()}, ln)

	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	require.Error(t, <-done, "Run must surface the Serve failure instead of swallowing it")
}

func TestWarnHaltedBoot_WarnsOnlyOfFailuresBesidesTheHalt(t *testing.T) {
	halt := &migrate.Halt{Reason: migrate.HaltStepFailed, Message: "step 1 of 5 failed"}
	closeFailure := errors.New("injected close failure")
	warnings := testutil.CaptureLog(t, testutil.AtLeast(slog.LevelWarn))

	warnHaltedBoot(errors.Join(fmt.Errorf("internal: app: %w", errors.Join(fmt.Errorf("app: migrate: %w", halt)))))
	require.Empty(t, warnings, "the halt alone")
	warnHaltedBoot(errors.Join(fmt.Errorf("internal: app: %w", halt), closeFailure))

	require.Len(t, warnings, 1)
	logged, ok := testutil.Attrs(<-warnings)["err"].Any().(error)
	require.True(t, ok, "the warning carries the boot's error")
	require.ErrorIs(t, logged, closeFailure)
	require.ErrorIs(t, logged, halt)
}

func TestRun_ServesEveryListener(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	unixLn, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unixLn.Close() })
	tcpLn, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	c := serving(t, &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})}, unixLn, tcpLn)
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

func TestRun_ClosesAnIdleKeepAliveConnection(t *testing.T) {
	tcpLn, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	c := serving(t, newHTTPServer(http.HandlerFunc(echoOrOK), 50*time.Millisecond), tcpLn)
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

func serving(
	t *testing.T,
	server *http.Server,
	listeners ...net.Listener,
) *Container {
	t.Helper()
	hooks, err := hookserver.Listen(context.Background(), http.NewServeMux())
	require.NoError(t, err)
	t.Cleanup(func() { _ = hooks.Close() })
	c := &Container{
		server:    server,
		listeners: listeners,
		serveErr:  make(chan error, len(listeners)+1),
		stack:     &stack{hooks: hooks},
	}
	for _, ln := range listeners {
		c.serve(func() error { return server.Serve(ln) })
	}
	return c
}

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
