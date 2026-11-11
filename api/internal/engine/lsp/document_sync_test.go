package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// LSP requires a strictly increasing version on every didChange; a constant
// version makes servers drop or reject edits after the first one.
func TestDidChange_VersionsIncreaseAndResetOnReopen(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	version := func(c call) any {
		return as[map[string]any](t, as[map[string]any](t, c.params)["textDocument"])["version"]
	}

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "a"))
	require.NoError(t, e.DidChange(ctx, ws, tree, goF, "ab"))
	require.NoError(t, e.DidChange(ctx, ws, tree, goF, "abc"))
	require.NoError(t, e.DidClose(ctx, ws, tree, goF))
	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "x"))
	require.NoError(t, e.DidChange(ctx, ws, tree, goF, "xy"))

	nots := fake.notifies()
	require.Len(t, nots, 6)
	assert.Equal(t, 1, version(nots[0]))
	assert.Equal(t, 2, version(nots[1]))
	assert.Equal(t, 3, version(nots[2]))
	assert.Equal(t, 1, version(nots[4]))
	assert.Equal(t, 2, version(nots[5]))
}

func TestDidSave_RidesTheOpenDocumentsServerOnly(t *testing.T) {
	fake := newFakeServer(nil)
	e, spawns := buildCountingEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidSave(ctx, ws, tree, goF))
	assert.Equal(t, 0, spawns.count(), "didSave must not spawn a server")
	assert.Empty(t, fake.notifies())

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "a"))
	require.NoError(t, e.DidSave(ctx, ws, tree, goF))
	nots := fake.notifies()
	require.Len(t, nots, 2)
	assert.Equal(t, "textDocument/didSave", nots[1].method)

	require.NoError(t, e.DidClose(ctx, ws, tree, goF))
	assert.Equal(t, 1, fake.closeCount(), "didSave must not leak a ref")
}

func TestDidOpen_ForwardsAndTracksURI(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	err := e.DidOpen(context.Background(), ws, tree, goF, "go", "package main")
	require.NoError(t, err)

	nots := fake.notifies()
	require.Len(t, nots, 1)
	assert.Equal(t, "textDocument/didOpen", nots[0].method)
}

func TestDidOpen_RefusesADocumentOutsideTheWorktree(t *testing.T) {
	for _, path := range []string{"/elsewhere/main.go", "../sibling/main.go", "/tree/../other/main.go"} {
		fake := newFakeServer(nil)
		e, spawns := buildCountingEngine(t, fake)

		err := e.DidOpen(context.Background(), ws, tree, path, "go", "package main")
		require.ErrorIs(t, err, problem.ErrInvalidArgument, path)
		assert.Empty(t, fake.notifies(), "%s: nothing reaches the server", path)
		assert.Equal(t, 0, spawns.count(), "%s: no server is spawned for it", path)
	}
}

func TestDidChange_Forwards(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	err := e.DidChange(context.Background(), ws, tree, goF, "package main // edited")
	require.NoError(t, err)
	assert.Equal(t, "textDocument/didChange", fake.notifies()[0].method)
}

func TestDidClose_Forwards(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
	require.NoError(t, e.DidClose(ctx, ws, tree, goF))

	nots := fake.notifies()
	require.Len(t, nots, 2)
	assert.Equal(t, "textDocument/didOpen", nots[0].method)
	assert.Equal(t, "textDocument/didClose", nots[1].method)
}

func TestDidClose_WithoutOpenIsNoOp(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	require.NoError(t, e.DidClose(context.Background(), ws, tree, goF))
	assert.Empty(t, fake.notifies(), "didClose with no open document forwards nothing")
	assert.Equal(t, 0, fake.closeCount())
}

// TestEngine_OpenKeepsServerWarm_CloseTearsDown proves the document-lifetime
// model: DidOpen holds a ref that survives requests, and DidClose drops it,
// tearing the server down exactly once and firing OnReleaseEmpty (snapshot
// evicted).
func TestEngine_OpenKeepsServerWarm_CloseTearsDown(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"items":[]}`))
	e, sc := buildCountingEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))

	cb := fake.diagCallback()
	require.NotNil(t, cb)
	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "boom"}}})
	require.Len(t, e.DiagnosticsSnapshot(ws), 1)

	for i := 0; i < 3; i++ {
		_, err := e.Completion(ctx, ws, tree, goF, pos())
		require.NoError(t, err)
	}

	// Across the whole open session exactly one server was spawned and it stayed
	// alive: the open-doc ref kept it warm, so requests reused it.
	assert.Equal(t, 1, sc.count(), "the open document keeps a single server warm")
	assert.Equal(t, 0, fake.closeCount(), "server must not be torn down while the doc is open")

	require.NoError(t, e.DidClose(ctx, ws, tree, goF))

	assert.Equal(t, 1, sc.count(), "close spawns nothing new")
	assert.Equal(t, 1, fake.closeCount(), "close drops the last ref and tears the server down")
	assert.Empty(t, e.DiagnosticsSnapshot(ws), "OnReleaseEmpty evicts the snapshot")
	_, held := e.snap[ws]
	assert.False(t, held, "snapshot map must no longer hold the wsID key")
}

// TestEngine_DidChangeIsRefNeutral proves didChange does not permanently change
// the refcount: with an open document the server stays warm (one spawn, no
// teardown) until DidClose, and the change itself neither spawns nor closes.
func TestEngine_DidChangeIsRefNeutral(t *testing.T) {
	fake := newFakeServer(nil)
	e, sc := buildCountingEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))

	for i := 0; i < 3; i++ {
		require.NoError(t, e.DidChange(ctx, ws, tree, goF, "package main // edit"))
	}

	assert.Equal(t, 1, sc.count(), "didChange reuses the warm server")
	assert.Equal(t, 0, fake.closeCount(), "didChange must not tear the server down")

	require.NoError(t, e.DidClose(ctx, ws, tree, goF))
	assert.Equal(t, 1, fake.closeCount(), "only the matching close tears the server down")
}

func TestNotify_ServerErrorPropagates(t *testing.T) {
	fake := newFakeServer(nil)
	fake.notErr = errors.New("boom")
	e := buildEngine(t, fake)

	err := e.DidOpen(context.Background(), ws, tree, goF, "go", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/didOpen")
}

func TestNotify_SpawnErrorPropagates(t *testing.T) {
	e := buildFailingEngine(t)
	err := e.DidChange(context.Background(), ws, tree, goF, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/didChange")
}
