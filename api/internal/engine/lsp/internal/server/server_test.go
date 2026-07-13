package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

var errBoom = errors.New("boom")

// fakeServer is an in-process LSP peer speaking the protocol framing over one
// end of a net.Pipe. It records received requests/notifications and lets the
// test drive responses and notifications.
type fakeServer struct {
	conn   net.Conn
	reader *bufio.Reader

	mu       sync.Mutex
	requests []protocol.Request
	notifs   []protocol.Notification

	gotReq   chan protocol.Request
	gotNotif chan protocol.Notification
}

func newFakeServer(
	conn net.Conn,
) *fakeServer {
	f := &fakeServer{
		conn:     conn,
		reader:   bufio.NewReader(conn),
		gotReq:   make(chan protocol.Request, 16),
		gotNotif: make(chan protocol.Notification, 16),
	}
	go f.readLoop()
	return f
}

func (f *fakeServer) readLoop() {
	for {
		payload, err := protocol.ReadMessage(f.reader)
		if err != nil {
			return
		}
		f.handle(payload)
	}
}

func (f *fakeServer) handle(
	payload []byte,
) {
	var probe struct {
		ID *int `json:"id"`
	}
	_ = json.Unmarshal(payload, &probe)

	if probe.ID == nil {
		var n protocol.Notification
		_ = json.Unmarshal(payload, &n)
		f.mu.Lock()
		f.notifs = append(f.notifs, n)
		f.mu.Unlock()
		f.gotNotif <- n
		return
	}

	var r protocol.Request
	_ = json.Unmarshal(payload, &r)
	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.mu.Unlock()
	f.gotReq <- r
}

func (f *fakeServer) respond(
	id int,
	result any,
) {
	raw, _ := json.Marshal(result)
	resp := protocol.Response{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  raw,
	}
	out, _ := json.Marshal(resp)
	_ = protocol.WriteMessage(f.conn, out)
}

func (f *fakeServer) pushDiagnostics(
	uri string,
	diags []map[string]any,
) {
	params, _ := json.Marshal(map[string]any{
		"uri":         uri,
		"diagnostics": diags,
	})
	resp := protocol.Response{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  params,
	}
	out, _ := json.Marshal(resp)
	_ = protocol.WriteMessage(f.conn, out)
}

func (f *fakeServer) notifCount(
	method string,
) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, notif := range f.notifs {
		if notif.Method == method {
			n++
		}
	}
	return n
}

// newTestServer wires a server over one end of a net.Pipe and returns the
// server plus the fake peer on the other end. The spawn closure reconnects
// the same fake on Replay by handing back a fresh pipe whose far end is
// served by a new fakeServer recorded into fakeRef.
func newTestServer(
	t *testing.T,
) (Server, *fakeServer) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	fake := newFakeServer(serverConn)

	srv := newOverTransport(clientConn, nil)
	t.Cleanup(func() { _ = srv.Close() })
	return srv, fake
}

func TestServer_InitializeHandshake(t *testing.T) {
	srv, fake := newTestServer(t)

	done := make(chan error, 1)
	go func() {
		done <- srv.Initialize(context.Background(), "/tmp/proj")
	}()

	req := <-fake.gotReq
	assert.Equal(t, "initialize", req.Method)

	var params map[string]any
	require.NoError(t, json.Unmarshal(req.Params, &params))
	assert.Equal(t, "file:///tmp/proj", params["rootUri"])

	fake.respond(req.ID, map[string]any{"capabilities": map[string]any{}})

	notif := <-fake.gotNotif
	assert.Equal(t, "initialized", notif.Method)

	require.NoError(t, <-done)
}

func TestServer_RequestCorrelatesResult(t *testing.T) {
	srv, fake := newTestServer(t)

	type result struct {
		raw json.RawMessage
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := srv.Request(context.Background(), "textDocument/hover", map[string]any{"x": 1})
		done <- result{raw, err}
	}()

	req := <-fake.gotReq
	assert.Equal(t, "textDocument/hover", req.Method)
	fake.respond(req.ID, map[string]any{"contents": "doc"})

	got := <-done
	require.NoError(t, got.err)
	assert.JSONEq(t, `{"contents":"doc"}`, string(got.raw))
}

func TestServer_RequestReturnsRPCError(t *testing.T) {
	srv, fake := newTestServer(t)

	done := make(chan error, 1)
	go func() {
		_, err := srv.Request(context.Background(), "bad", nil)
		done <- err
	}()

	req := <-fake.gotReq
	id := req.ID
	resp := protocol.Response{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &protocol.RPCError{Code: -32601, Message: "method not found"},
	}
	out, _ := json.Marshal(resp)
	require.NoError(t, protocol.WriteMessage(fake.conn, out))

	err := <-done
	require.Error(t, err)
	assert.Contains(t, err.Error(), "method not found")
}

func TestServer_DiagnosticsCallback(t *testing.T) {
	srv, fake := newTestServer(t)

	got := make(chan lsp.DiagnosticsEvent, 1)
	srv.OnDiagnostics(func(ev lsp.DiagnosticsEvent) {
		got <- ev
	})

	fake.pushDiagnostics("file:///main.go", []map[string]any{
		{
			"range": map[string]any{
				"start": map[string]any{"line": 1, "character": 2},
				"end":   map[string]any{"line": 1, "character": 5},
			},
			"severity": 1,
			"message":  "boom",
			"source":   "gopls",
		},
	})

	// Block on the callback the server actually invokes when it decodes the
	// notification off the wire — the real signal. `go test -timeout` is the
	// backstop if it never arrives.
	ev := <-got
	require.Len(t, ev.Diagnostics, 1)
	assert.Equal(t, "/main.go", ev.Diagnostics[0].FilePath)
	assert.Equal(t, "error", ev.Diagnostics[0].Severity)
	assert.Equal(t, "boom", ev.Diagnostics[0].Message)
}

func TestServer_NotifyTracksDidOpen(t *testing.T) {
	srv, fake := newTestServer(t)

	err := srv.Notify(context.Background(), "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": "file:///main.go"},
	})
	require.NoError(t, err)

	<-fake.gotNotif
	assert.Equal(t, []string{"file:///main.go"}, srv.OpenDocs().List())
}

func TestServer_ReplayReopensTrackedDocs(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	fake1 := newFakeServer(serverConn)

	var mu sync.Mutex
	fakes := []*fakeServer{fake1}

	spawn := func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		c, s := net.Pipe()
		mu.Lock()
		fakes = append(fakes, newFakeServer(s))
		mu.Unlock()
		return c, nil
	}

	srv := newOverTransport(clientConn, spawn)
	t.Cleanup(func() { _ = srv.Close() })

	require.NoError(t, srv.Notify(context.Background(), "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": "file:///main.go"},
	}))
	<-fake1.gotNotif

	require.NoError(t, srv.Replay(context.Background()))

	mu.Lock()
	fake2 := fakes[1]
	mu.Unlock()

	<-fake2.gotNotif
	assert.Equal(t, 1, fake2.notifCount("textDocument/didOpen"))
}

func (f *fakeServer) lastNotif(
	method string,
) (protocol.Notification, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.notifs) - 1; i >= 0; i-- {
		if f.notifs[i].Method == method {
			return f.notifs[i], true
		}
	}
	return protocol.Notification{}, false
}

// TestServer_ReplayResendsWellFormedDidOpen asserts that Replay re-sends the
// exact, well-formed didOpen params (languageId/version/text) that were
// originally forwarded — not a bare {textDocument:{uri}} synthesized one.
func TestServer_ReplayResendsWellFormedDidOpen(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	fake1 := newFakeServer(serverConn)

	var mu sync.Mutex
	fakes := []*fakeServer{fake1}
	spawn := func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		c, s := net.Pipe()
		mu.Lock()
		fakes = append(fakes, newFakeServer(s))
		mu.Unlock()
		return c, nil
	}

	srv := newOverTransport(clientConn, spawn)
	t.Cleanup(func() { _ = srv.Close() })

	original := map[string]any{
		"textDocument": map[string]any{
			"uri":        "file:///main.go",
			"languageId": "go",
			"version":    1,
			"text":       "package main",
		},
	}
	require.NoError(t, srv.Notify(context.Background(), "textDocument/didOpen", original))
	<-fake1.gotNotif

	require.NoError(t, srv.Replay(context.Background()))

	mu.Lock()
	fake2 := fakes[1]
	mu.Unlock()
	<-fake2.gotNotif

	got, ok := fake2.lastNotif("textDocument/didOpen")
	require.True(t, ok)

	want, _ := json.Marshal(original)
	assert.JSONEq(t, string(want), string(got.Params))
}

// TestServer_ReplayFailsInFlightRequest asserts that a Request blocked when
// Replay swaps the transport returns promptly with ErrReplaced instead of
// hanging until its ctx cancels.
func TestServer_ReplayFailsInFlightRequest(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	fake1 := newFakeServer(serverConn)

	spawn := func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		c, s := net.Pipe()
		newFakeServer(s)
		return c, nil
	}

	srv := newOverTransport(clientConn, spawn)
	t.Cleanup(func() { _ = srv.Close() })

	done := make(chan error, 1)
	go func() {
		_, err := srv.Request(context.Background(), "slow", nil)
		done <- err
	}()
	<-fake1.gotReq // ensure the waiter is registered before replay

	require.NoError(t, srv.Replay(context.Background()))

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReplaced)
}

func TestServer_CloseThenRequestErrors(t *testing.T) {
	srv, _ := newTestServer(t)

	require.NoError(t, srv.Close())

	_, err := srv.Request(context.Background(), "textDocument/hover", nil)
	require.Error(t, err)
}

func TestServer_RequestRespectsContextCancel(t *testing.T) {
	srv, fake := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := srv.Request(ctx, "textDocument/hover", nil)
		done <- err
	}()

	// The fake server receiving the request is the real signal that the in-flight
	// waiter is registered — cancelling before that would exercise the early-bail
	// path instead. A 50ms deadline was only ever a guess that we had got here.
	<-fake.gotReq
	cancel()

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestServer_NotifyAfterCloseErrors(t *testing.T) {
	srv, _ := newTestServer(t)
	require.NoError(t, srv.Close())

	err := srv.Notify(context.Background(), "textDocument/didClose", nil)
	require.Error(t, err)
}

func TestServer_ReplayWithoutSpawnErrors(t *testing.T) {
	srv, _ := newTestServer(t)
	err := srv.Replay(context.Background())
	require.Error(t, err)
}

func TestServer_RequestRejectsUnmarshalableParams(t *testing.T) {
	srv, _ := newTestServer(t)
	_, err := srv.Request(context.Background(), "x", make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

func TestServer_NotifyRejectsUnmarshalableParams(t *testing.T) {
	srv, _ := newTestServer(t)
	err := srv.Notify(context.Background(), "x", make(chan int))
	require.Error(t, err)
}

func TestServer_NotifyRespectsContextCancel(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := srv.Notify(ctx, "x", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestServer_NotifyDidCloseUntracks(t *testing.T) {
	srv, fake := newTestServer(t)

	require.NoError(t, srv.Notify(context.Background(), "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": "file:///a.go"},
	}))
	<-fake.gotNotif
	require.NoError(t, srv.Notify(context.Background(), "textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": "file:///a.go"},
	}))
	<-fake.gotNotif

	assert.Empty(t, srv.OpenDocs().List())
}

func TestServer_DispatchIgnoresMalformedAndUnknownID(t *testing.T) {
	srv, fake := newTestServer(t)

	// Malformed JSON payload — dispatch must ignore it without panicking.
	require.NoError(t, protocol.WriteMessage(fake.conn, []byte("not json")))

	// Response for an id with no waiter — must be dropped silently.
	id := 9999
	resp := protocol.Response{JSONRPC: "2.0", ID: &id, Result: json.RawMessage(`{}`)}
	out, _ := json.Marshal(resp)
	require.NoError(t, protocol.WriteMessage(fake.conn, out))

	// A subsequent correlated request still works, proving the loop survived.
	done := make(chan json.RawMessage, 1)
	go func() {
		raw, _ := srv.Request(context.Background(), "ping", nil)
		done <- raw
	}()
	req := <-fake.gotReq
	fake.respond(req.ID, map[string]any{"pong": true})
	got := <-done
	assert.JSONEq(t, `{"pong":true}`, string(got))
}

func TestServer_DiagnosticsIgnoredWithoutCallback(t *testing.T) {
	srv, fake := newTestServer(t)

	// No OnDiagnostics registered — push must be a no-op.
	fake.pushDiagnostics("file:///x.go", []map[string]any{{"severity": 1, "message": "m"}})

	// Then a bad-params publishDiagnostics with a callback set must not panic.
	srv.OnDiagnostics(func(lsp.DiagnosticsEvent) {})
	resp := protocol.Response{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  json.RawMessage(`not json`),
	}
	out, _ := json.Marshal(resp)
	require.NoError(t, protocol.WriteMessage(fake.conn, out))

	// Liveness check.
	done := make(chan json.RawMessage, 1)
	go func() {
		raw, _ := srv.Request(context.Background(), "ping", nil)
		done <- raw
	}()
	req := <-fake.gotReq
	fake.respond(req.ID, map[string]any{"ok": 1})
	<-done
}

func TestServer_CloseIsIdempotent(t *testing.T) {
	srv, _ := newTestServer(t)
	require.NoError(t, srv.Close())
	require.NoError(t, srv.Close())
}

type errTransport struct {
	writeErr error
}

func (e *errTransport) Read(
	p []byte,
) (int, error) {
	select {} // block forever; the read loop just parks
}

func (e *errTransport) Write(
	p []byte,
) (int, error) {
	return 0, e.writeErr
}

func (e *errTransport) Close() error {
	return nil
}

func TestServer_WritePropagatesTransportError(t *testing.T) {
	srv := newOverTransport(&errTransport{writeErr: errBoom}, nil)
	t.Cleanup(func() { _ = srv.Close() })

	err := srv.Notify(context.Background(), "x", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write")
}

func TestServer_ReplayPropagatesSpawnError(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	newFakeServer(serverConn)

	spawn := func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		return nil, errBoom
	}
	srv := newOverTransport(clientConn, spawn)
	t.Cleanup(func() { _ = srv.Close() })

	err := srv.Replay(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spawn")
}

func TestServer_NotifyDidOpenWithoutURIIsIgnored(t *testing.T) {
	srv, fake := newTestServer(t)

	require.NoError(t, srv.Notify(context.Background(), "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{},
	}))
	<-fake.gotNotif
	assert.Empty(t, srv.OpenDocs().List())
}

func TestServer_NewFailsOnBadCommand(t *testing.T) {
	_, err := New("this-binary-does-not-exist-crowbar", nil, "")
	require.Error(t, err)
}

func TestServer_EmptyDiagnosticsDeliversEmptyBatch(t *testing.T) {
	srv, fake := newTestServer(t)
	got := make(chan lsp.DiagnosticsEvent, 1)
	srv.OnDiagnostics(func(ev lsp.DiagnosticsEvent) { got <- ev })

	fake.pushDiagnostics("file:///x.go", nil)

	// Block on the real callback; an empty batch is still a delivered batch.
	ev := <-got
	assert.Empty(t, ev.Diagnostics)
}

func TestServer_PendingRequestFailsOnClose(t *testing.T) {
	srv, fake := newTestServer(t)

	done := make(chan error, 1)
	go func() {
		_, err := srv.Request(context.Background(), "slow", nil)
		done <- err
	}()
	<-fake.gotReq // ensure the waiter is registered before closing

	require.NoError(t, srv.Close())

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrClosed)
}
