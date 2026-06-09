package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

// fakeServer records the methods it was asked to Request/Notify and returns a
// canned result for the next Request.
type fakeServer struct {
	mu       sync.Mutex
	result   json.RawMessage
	reqErr   error
	notErr   error
	reqCalls []call
	notCalls []call
	diagFn   func(domlsp.DiagnosticsEvent)
	closedN  int
	docs     *server.OpenDocs
}

type call struct {
	method string
	params any
}

func newFakeServer(
	result json.RawMessage,
) *fakeServer {
	return &fakeServer{result: result, docs: server.NewOpenDocs()}
}

func (f *fakeServer) Request(
	_ context.Context,
	method string,
	params any,
) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqCalls = append(f.reqCalls, call{method: method, params: params})
	return f.result, f.reqErr
}

func (f *fakeServer) Notify(
	_ context.Context,
	method string,
	params any,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notCalls = append(f.notCalls, call{method: method, params: params})
	return f.notErr
}

func (f *fakeServer) Initialize(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (f *fakeServer) OnDiagnostics(
	fn func(domlsp.DiagnosticsEvent),
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.diagFn = fn
}

func (f *fakeServer) OpenDocs() *server.OpenDocs {
	return f.docs
}

func (f *fakeServer) Replay(
	_ context.Context,
) error {
	return nil
}

func (f *fakeServer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closedN++
	return nil
}

func (f *fakeServer) requests() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.reqCalls...)
}

func (f *fakeServer) notifies() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.notCalls...)
}

func (f *fakeServer) diagCallback() func(domlsp.DiagnosticsEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.diagFn
}

func (f *fakeServer) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closedN
}

// buildEngine returns an Engine over a real manager whose spawn always yields
// the given fake server, plus an accessor for the spawned fake. A nil fake
// means the registry has no spec for the file (graceful-absence path) — set up
// by using an extension absent from the registry in the test.
func buildEngine(
	t *testing.T,
	fake *fakeServer,
) *engine {
	t.Helper()
	reg := registry.New(nil)
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return fake, nil
	}
	return newWithManager(reg, manager.New(reg, spawn, foundLookPath()))
}

// spawnCounter counts how many times the manager asked to spawn a fresh server
// process. Combined with the fake's closeCount it makes server teardown
// observable: a balanced session has spawns == closes and a pool that drains
// back to empty (the manager fires OnReleaseEmpty, evicting the snapshot).
type spawnCounter struct {
	mu sync.Mutex
	n  int
}

func (s *spawnCounter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// buildCountingEngine returns an engine whose spawn always yields fake while
// counting spawns. fake.closeCount() reports teardowns, so ref-balance tests can
// assert spawn/teardown symmetry without reaching into manager internals.
func buildCountingEngine(
	t *testing.T,
	fake *fakeServer,
) (*engine, *spawnCounter) {
	t.Helper()
	sc := &spawnCounter{}
	reg := registry.New(nil)
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		sc.mu.Lock()
		sc.n++
		sc.mu.Unlock()
		return fake, nil
	}
	return newWithManager(reg, manager.New(reg, spawn, foundLookPath())), sc
}

// foundLookPath reports every server binary as installed so engine tests
// exercise the spawn path without depending on real binaries on PATH.
func foundLookPath() manager.Option {
	return manager.WithLookPath(func(command string) (string, error) {
		return "/usr/bin/" + command, nil
	})
}

const (
	ws   = "ws1"
	tree = "/tree"
	goF  = "/tree/main.go"
	noF  = "/tree/notes.unknownext"
)

func pos() domlsp.Position {
	return domlsp.Position{Line: 2, Character: 5}
}

func TestCompletion_ForwardsAndPassesThrough(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"items":[{"label":"Foo"}]}`))
	e := buildEngine(t, fake)

	got, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[{"label":"Foo"}]}`, string(got))

	reqs := fake.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "textDocument/completion", reqs[0].method)
}

func TestHover_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"contents":"doc"}`))
	e := buildEngine(t, fake)

	got, err := e.Hover(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	assert.JSONEq(t, `{"contents":"doc"}`, string(got))
	assert.Equal(t, "textDocument/hover", fake.requests()[0].method)
}

func TestDefinition_ConvertsLocations(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///tree/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.Definition(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, "/tree/a.go", locs[0].FilePath)
	assert.Equal(t, "textDocument/definition", fake.requests()[0].method)
}

func TestReferences_ConvertsLocations(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///tree/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}},
		  {"uri":"file:///tree/b.go","range":{"start":{"line":2,"character":1},"end":{"line":2,"character":3}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.References(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 2)
	assert.Equal(t, "textDocument/references", fake.requests()[0].method)
}

func TestRename_ConvertsWorkspaceEdit(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`{"changes":{"file:///tree/a.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":3}},"newText":"X"}]}}`,
	))
	e := buildEngine(t, fake)

	we, err := e.Rename(context.Background(), ws, tree, goF, pos(), "X")
	require.NoError(t, err)
	edits, ok := we.Changes["/tree/a.go"]
	require.True(t, ok)
	require.Len(t, edits, 1)
	assert.Equal(t, "X", edits[0].NewText)

	reqs := fake.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "textDocument/rename", reqs[0].method)
}

func TestCodeAction_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[{"title":"Fix"}]`))
	e := buildEngine(t, fake)

	rng := domlsp.Range{Start: domlsp.Position{Line: 1}, End: domlsp.Position{Line: 2}}
	got, err := e.CodeAction(context.Background(), ws, tree, goF, rng)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"title":"Fix"}]`, string(got))
	assert.Equal(t, "textDocument/codeAction", fake.requests()[0].method)
}

func TestDocumentSymbol_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[{"name":"Foo","kind":12}]`))
	e := buildEngine(t, fake)

	got, err := e.DocumentSymbol(context.Background(), ws, tree, goF)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"Foo","kind":12}]`, string(got))
	assert.Equal(t, "textDocument/documentSymbol", fake.requests()[0].method)
}

// --- Document sync ---

func TestAbsFilePath(t *testing.T) {
	assert.Equal(t, "/tree/main.go", absFilePath("/tree", "main.go"))
	assert.Equal(t, "/tree/pkg/util.go", absFilePath("/tree", "pkg/util.go"))
	assert.Equal(t, "/already/abs.go", absFilePath("/tree", "/already/abs.go"))
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

// --- Graceful absence ---

func TestGracefulAbsence_FeaturesReturnEmptyNilError(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"items":[]}`))
	e := buildEngine(t, fake)
	ctx := context.Background()

	comp, err := e.Completion(ctx, ws, tree, noF, pos())
	require.NoError(t, err)
	assert.Nil(t, comp)

	hov, err := e.Hover(ctx, ws, tree, noF, pos())
	require.NoError(t, err)
	assert.Nil(t, hov)

	def, err := e.Definition(ctx, ws, tree, noF, pos())
	require.NoError(t, err)
	assert.Empty(t, def)

	refs, err := e.References(ctx, ws, tree, noF, pos())
	require.NoError(t, err)
	assert.Empty(t, refs)

	we, err := e.Rename(ctx, ws, tree, noF, pos(), "X")
	require.NoError(t, err)
	assert.Empty(t, we.Changes)

	ca, err := e.CodeAction(ctx, ws, tree, noF, domlsp.Range{})
	require.NoError(t, err)
	assert.Nil(t, ca)

	ds, err := e.DocumentSymbol(ctx, ws, tree, noF)
	require.NoError(t, err)
	assert.Nil(t, ds)

	assert.Empty(t, fake.requests(), "no server should have been spawned")
}

// buildAbsentBinaryEngine returns an engine whose registry has a spec for .go
// but whose LookPath reports the binary missing, so ServerForFile returns
// ErrNoServer (binary-not-installed graceful absence, 10 §5).
func buildAbsentBinaryEngine(
	t *testing.T,
) *engine {
	t.Helper()
	reg := registry.New(nil)
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		t.Fatal("spawn must not be called when the binary is absent")
		return nil, nil
	}
	mgr := manager.New(reg, spawn, manager.WithLookPath(func(string) (string, error) {
		return "", exec.ErrNotFound
	}))
	return newWithManager(reg, mgr)
}

func TestGracefulAbsence_MissingBinaryReturnsEmptyNilError(t *testing.T) {
	e := buildAbsentBinaryEngine(t)
	ctx := context.Background()

	comp, err := e.Completion(ctx, ws, tree, goF, pos())
	require.NoError(t, err)
	assert.Nil(t, comp)

	def, err := e.Definition(ctx, ws, tree, goF, pos())
	require.NoError(t, err)
	assert.Empty(t, def)

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
}

func TestGracefulAbsence_SyncIsNoOp(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, noF, "x", "txt"))
	require.NoError(t, e.DidChange(ctx, ws, tree, noF, "txt"))
	require.NoError(t, e.DidClose(ctx, ws, tree, noF))
	assert.Empty(t, fake.notifies())
}

// --- Diagnostics snapshot ---

func TestDiagnosticsSnapshot_EmptyUntilEvent(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	assert.Empty(t, e.DiagnosticsSnapshot(ws))

	// Spawn the server so its diagnostics callback is wired through the manager.
	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)

	cb := fake.diagCallback()
	require.NotNil(t, cb)

	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "boom"}}})

	snap := e.DiagnosticsSnapshot(ws)
	require.Len(t, snap, 1)
	assert.Equal(t, "boom", snap[0].Message)
	assert.Empty(t, e.DiagnosticsSnapshot("other"))
}

func TestOnDiagnostics_UserCallbackAlsoInvoked(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	var got domlsp.DiagnosticsEvent
	e.OnDiagnostics(func(ev domlsp.DiagnosticsEvent) { got = ev })

	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)

	cb := fake.diagCallback()
	require.NotNil(t, cb)
	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "x"}}})

	assert.Equal(t, ws, got.WsID)
	require.Len(t, got.Diagnostics, 1)
}

// --- Release ---

func TestRelease_ClosesServerHeldByOpenDoc(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	// DidOpen holds a ref; the explicit Release drops it and tears the server
	// down. (Release is the public seam the WS layer uses on subscription drop.)
	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
	assert.Equal(t, 0, fake.closeCount(), "open document must keep the server warm")

	e.Release(ctx, ws, goF)
	assert.Equal(t, 1, fake.closeCount())
}

func TestRelease_NoSpecIsNoOp(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	e.Release(context.Background(), ws, noF)
	assert.Equal(t, 0, fake.closeCount())
}

// --- Refcount balance (the leak the fix closes) ---

// TestEngine_FeatureRequestsAreRefNeutral proves a feature request with NO open
// document is net-zero: each request spawns the server, serves, and tears it
// down. Before the fix the refcount grew by one per request and the server was
// never closed.
func TestEngine_FeatureRequestsAreRefNeutral(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"items":[]}`))
	e, sc := buildCountingEngine(t, fake)
	ctx := context.Background()

	const n = 4
	for i := 0; i < n; i++ {
		_, err := e.Completion(ctx, ws, tree, goF, pos())
		require.NoError(t, err)
	}

	// No open doc: each request must balance — spawn then teardown — so the
	// counts grow together and the pool never retains a leaked ref.
	assert.Equal(t, n, sc.count(), "each request spawns a fresh warm server")
	assert.Equal(t, n, fake.closeCount(), "each request tears the server down again")
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

// --- Error propagation ---

func TestRequest_ServerErrorPropagates(t *testing.T) {
	fake := newFakeServer(nil)
	fake.reqErr = errors.New("boom")
	e := buildEngine(t, fake)

	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/completion")
}

func TestNotify_ServerErrorPropagates(t *testing.T) {
	fake := newFakeServer(nil)
	fake.notErr = errors.New("boom")
	e := buildEngine(t, fake)

	err := e.DidOpen(context.Background(), ws, tree, goF, "go", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/didOpen")
}

func buildFailingEngine(
	t *testing.T,
) *engine {
	t.Helper()
	reg := registry.New(nil)
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return nil, errors.New("spawn failed")
	}
	return newWithManager(reg, manager.New(reg, spawn, foundLookPath()))
}

func TestRequest_SpawnErrorPropagates(t *testing.T) {
	e := buildFailingEngine(t)
	_, err := e.Definition(context.Background(), ws, tree, goF, pos())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/definition")
}

func TestNotify_SpawnErrorPropagates(t *testing.T) {
	e := buildFailingEngine(t)
	err := e.DidChange(context.Background(), ws, tree, goF, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/didChange")
}

func TestSpawnProcess_BadCommandErrors(t *testing.T) {
	_, err := spawnProcess(
		context.Background(),
		registry.ServerSpec{Command: "crowbar-no-such-lsp-binary-xyz"},
		t.TempDir(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: spawn")
}

// blockingServer is a server.Server whose Request never answers on its own; it
// returns only when the passed ctx is cancelled, modelling a wedged gopls.
type blockingServer struct {
	*fakeServer
}

func (b *blockingServer) Request(
	ctx context.Context,
	_ string,
	_ any,
) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRequest_TimesOutOnWedgedServer(t *testing.T) {
	fake := &blockingServer{fakeServer: newFakeServer(nil)}
	reg := registry.New(nil)
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return fake, nil
	}
	e := newWithManager(reg, manager.New(reg, spawn, foundLookPath()))
	e.reqTimeout = time.Millisecond

	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestEngine_DefaultRequestTimeoutIsApplied(t *testing.T) {
	e := New(nil).(*engine)
	assert.Equal(t, DefaultRequestTimeout, e.reqTimeout)
}

func TestNew_ReturnsEngine(t *testing.T) {
	got := New(nil)
	assert.NotNil(t, got)
}

func TestNew_WithOverrides(t *testing.T) {
	got := New(map[string]registry.ServerSpec{
		".go": {Command: "gopls-x", LanguageID: "go", Extensions: []string{".go"}},
	})
	assert.NotNil(t, got)
}
