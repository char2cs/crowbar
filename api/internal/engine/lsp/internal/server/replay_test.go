package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

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

func TestServer_ReplayWithoutSpawnErrors(t *testing.T) {
	srv, _ := newTestServer(t)
	err := srv.Replay(context.Background())
	require.Error(t, err)
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

// TestServer_ReplayRepeatsHandshakeAndReopensLatestText pins "restart
// language server": the respawned process gets initialize/initialized before
// any didOpen (a server ignores everything before initialize), and the
// reopened text is the latest didChange, not the text the file was opened
// with.
func TestServer_ReplayRepeatsHandshakeAndReopensLatestText(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	fake1 := newFakeServer(serverConn)

	fakes := make(chan *fakeServer, 1)
	spawn := func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		c, s := net.Pipe()
		fakes <- newFakeServer(s)
		return c, nil
	}

	srv := newOverTransport(clientConn, spawn)
	t.Cleanup(func() { _ = srv.Close() })

	initDone := make(chan error, 1)
	go func() { initDone <- srv.Initialize(context.Background(), "/repo") }()
	req := <-fake1.gotReq
	fake1.respond(req.ID, map[string]any{"capabilities": map[string]any{}})
	require.NoError(t, <-initDone)
	<-fake1.gotNotif // initialized

	ctx := context.Background()
	require.NoError(t, srv.Notify(ctx, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": "file:///repo/main.go", "languageId": "go", "version": 1, "text": "v1",
		},
	}))
	<-fake1.gotNotif
	require.NoError(t, srv.Notify(ctx, "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": "file:///repo/main.go", "version": 7},
		"contentChanges": []any{map[string]any{"text": "v2"}},
	}))
	<-fake1.gotNotif

	replayDone := make(chan error, 1)
	go func() { replayDone <- srv.Replay(ctx) }()
	fake2 := <-fakes
	initReq := <-fake2.gotReq
	assert.Equal(t, "initialize", initReq.Method)
	fake2.respond(initReq.ID, map[string]any{"capabilities": map[string]any{}})
	require.NoError(t, <-replayDone)

	first := <-fake2.gotNotif
	assert.Equal(t, "initialized", first.Method)
	reopen := <-fake2.gotNotif
	assert.Equal(t, "textDocument/didOpen", reopen.Method)
	assert.JSONEq(t,
		`{"textDocument":{"uri":"file:///repo/main.go","languageId":"go","version":7,"text":"v2"}}`,
		string(reopen.Params))
}
