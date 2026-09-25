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

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
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
	exitFn   func()
	closedN  int
	replayN  int
	docs     *server.OpenDocs
	// byMethod answers a Request for that method instead of result.
	byMethod map[string]json.RawMessage
	// errByMethod fails a Request for that method.
	errByMethod map[string]error
	semTok      semtok.Support
	// commands are the commands CanExecute accepts.
	commands map[string]bool
	// cmdEdits are the applyEdit requests ExecuteCommand reports.
	cmdEdits []json.RawMessage
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
	if err, ok := f.errByMethod[method]; ok {
		return nil, err
	}
	if raw, ok := f.byMethod[method]; ok {
		return raw, f.reqErr
	}
	return f.result, f.reqErr
}

func (f *fakeServer) SemanticTokens() semtok.Support {
	return f.semTok
}

func (f *fakeServer) CanExecute(
	command string,
) bool {
	return f.commands[command]
}

func (f *fakeServer) ExecuteCommand(
	ctx context.Context,
	params any,
) (json.RawMessage, []json.RawMessage, error) {
	result, err := f.Request(ctx, "workspace/executeCommand", params)
	return result, f.cmdEdits, err
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

func (f *fakeServer) OnExit(
	fn func(),
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exitFn = fn
}

func (f *fakeServer) OpenDocs() *server.OpenDocs {
	return f.docs
}

func (f *fakeServer) Replay(
	_ context.Context,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replayN++
	return nil
}

func (f *fakeServer) replayCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.replayN
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
	assert.Equal(t, "a.go", locs[0].FilePath)
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
	edits, ok := we.Changes["a.go"]
	require.True(t, ok)
	require.Len(t, edits, 1)
	assert.Equal(t, "X", edits[0].NewText)

	reqs := fake.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "textDocument/rename", reqs[0].method)
}

func TestCodeAction_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[{"title":"Fix","edit":{"changes":{}}}]`))
	e := buildEngine(t, fake)

	rng := domlsp.Range{Start: domlsp.Position{Line: 1}, End: domlsp.Position{Line: 2}}
	got, err := e.CodeAction(context.Background(), ws, tree, goF, rng, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"title":"Fix","edit":{"changes":{}}}]`, string(got))
	assert.Equal(t, "textDocument/codeAction", fake.requests()[0].method)
}

func TestCodeAction_ForwardsDiagnosticsAndRelativizesEdits(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[
		{"title":"Fix","kind":"quickfix","edit":{"changes":{"file:///tree/pkg/a.go":[
			{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"x"}
		]}}},
		{"title":"Organize","edit":{"documentChanges":[
			{"textDocument":{"uri":"file:///tree/main.go","version":3},"edits":[]}
		]}},
		{"title":"Run","command":"gopls.run"},
		{"title":"Show","command":"java.show.references"},
		{"title":"Both","edit":{"changes":{}},"command":{"title":"x","command":"client.only"}}
	]`))
	fake.commands = map[string]bool{"gopls.run": true}
	e := buildEngine(t, fake)
	diags := json.RawMessage(`[{"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}},"message":"unused"}]`)

	got, err := e.CodeAction(context.Background(), ws, tree, goF, domlsp.Range{}, diags)
	require.NoError(t, err)

	params := as[map[string]any](t, fake.requests()[0].params)
	ctxParam := as[map[string]any](t, params["context"])
	assert.JSONEq(t, string(diags), string(as[json.RawMessage](t, ctxParam["diagnostics"])))

	assert.JSONEq(t, `[
		{"title":"Fix","kind":"quickfix","edit":{"changes":{"pkg/a.go":[
			{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"x"}
		]}}},
		{"title":"Organize","edit":{"documentChanges":[
			{"textDocument":{"uri":"main.go","version":3},"edits":[]}
		]}},
		{"title":"Run","command":"gopls.run"},
		{"title":"Both","edit":{"changes":{}}}
	]`, string(got))
}

func TestNewFeatureRequests_ForwardTheirMethods(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"ok":true}`))
	e := buildEngine(t, fake)
	ctx := context.Background()

	_, err := e.SignatureHelp(ctx, ws, tree, goF, pos())
	require.NoError(t, err)
	_, err = e.CodeLens(ctx, ws, tree, goF)
	require.NoError(t, err)
	_, err = e.CodeLensResolve(ctx, ws, tree, goF, json.RawMessage(`{"range":{}}`))
	require.NoError(t, err)
	_, err = e.Formatting(ctx, ws, tree, goF, domlsp.FormattingOptions{TabSize: 4, InsertSpaces: true})
	require.NoError(t, err)

	reqs := fake.requests()
	require.Len(t, reqs, 4)
	assert.Equal(t, "textDocument/signatureHelp", reqs[0].method)
	assert.Equal(t, "textDocument/codeLens", reqs[1].method)
	assert.Equal(t, "codeLens/resolve", reqs[2].method)
	assert.Equal(t, "textDocument/formatting", reqs[3].method)
	opts := as[map[string]any](t, as[map[string]any](t, reqs[3].params)["options"])
	assert.Equal(t, 4, opts["tabSize"])
	assert.Equal(t, true, opts["insertSpaces"])
}

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

func TestStatusAndRestart_ReflectTheRealServer(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	assert.Equal(t, domlsp.ServerUnsupported, e.Status(ws, noF).State)
	assert.Equal(t, domlsp.ServerStopped, e.Status(ws, goF).State)

	st, err := e.Restart(ctx, ws, goF)
	require.NoError(t, err)
	assert.Equal(t, domlsp.ServerStopped, st.State, "restart never spawns a stopped server")
	assert.Equal(t, 0, fake.replayCount())

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "a"))
	assert.Equal(t, domlsp.ServerRunning, e.Status(ws, goF).State)

	st, err = e.Restart(ctx, ws, goF)
	require.NoError(t, err)
	assert.Equal(t, domlsp.ServerRunning, st.State)
	assert.Equal(t, 1, fake.replayCount())
	assert.Equal(t, 0, fake.closeCount(), "restart keeps the pool entry and its refs")
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

func TestDidOpen_RefusesADocumentOutsideTheWorktree(t *testing.T) {
	for _, path := range []string{"/elsewhere/main.go", "../sibling/main.go", "/tree/../other/main.go"} {
		fake := newFakeServer(nil)
		e, spawns := buildCountingEngine(t, fake)

		err := e.DidOpen(context.Background(), ws, tree, path, "go", "package main")
		require.ErrorIs(t, err, apperr.ErrInvalidArgument, path)
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

	ca, err := e.CodeAction(ctx, ws, tree, noF, domlsp.Range{}, nil)
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

// TestShutdown_ClosesEverySpawnedServer proves R8: engine.Shutdown (called by
// engine.Container.Close on daemon shutdown) tears down every running language
// server regardless of refcount, so no gopls/tsserver survives the daemon.
func TestShutdown_ClosesEverySpawnedServer(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	// Open a document so a server is spawned and held warm (refcount > 0).
	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
	require.Equal(t, 0, fake.closeCount(), "open document keeps the server warm")

	e.Shutdown(ctx)
	assert.Equal(t, 1, fake.closeCount(), "Shutdown closes the live server despite its held ref")
}

// TestReleaseWorkspace_ReleasesAllServerRefs proves R11's engine side: releasing
// a workspace tears down every server it holds regardless of refcount and evicts
// the diagnostics snapshot, even though no per-document DidClose ran.
func TestReleaseWorkspace_ReleasesAllServerRefs(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
	cb := fake.diagCallback()
	require.NotNil(t, cb)
	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "boom"}}})
	require.Len(t, e.DiagnosticsSnapshot(ws), 1)

	// WS drop without DidClose: the open-doc ref is still held.
	require.Equal(t, 0, fake.closeCount())

	e.ReleaseWorkspace(ctx, ws)
	assert.Equal(t, 1, fake.closeCount(), "ReleaseWorkspace tears the server down despite the held ref")
	assert.Empty(t, e.DiagnosticsSnapshot(ws), "OnReleaseEmpty evicts the snapshot")
}

// TestReleaseWorkspace_UnknownWorkspaceIsNoOp verifies releasing a workspace
// with no running servers does not panic and tears nothing down.
func TestReleaseWorkspace_UnknownWorkspaceIsNoOp(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	e.ReleaseWorkspace(context.Background(), "ghost")
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

// as is a checked type assertion: it fails the test, naming the dynamic type, instead of
// panicking when v is not a T.
func as[T any](t *testing.T, v any) T {
	t.Helper()
	got, ok := v.(T)
	require.Truef(t, ok, "got %T, want %T", v, got)
	return got
}
