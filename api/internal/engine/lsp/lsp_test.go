package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

type fakeServer struct {
	mu          sync.Mutex
	result      json.RawMessage
	reqErr      error
	notErr      error
	reqCalls    []call
	notCalls    []call
	diagFn      func(domlsp.DiagnosticsEvent)
	exitFn      func()
	closedN     int
	replayN     int
	docs        *server.OpenDocs
	byMethod    map[string]json.RawMessage
	errByMethod map[string]error
	semTok      semtok.Support
	commands    map[string]bool
	cmdEdits    []json.RawMessage
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

type spawnCounter struct {
	mu sync.Mutex
	n  int
}

func (s *spawnCounter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

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

func TestRelease_ClosesServerHeldByOpenDoc(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

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

func TestShutdown_ClosesEverySpawnedServer(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
	require.Equal(t, 0, fake.closeCount(), "open document keeps the server warm")

	e.Shutdown(ctx)
	assert.Equal(t, 1, fake.closeCount(), "Shutdown closes the live server despite its held ref")
}

func TestReleaseWorkspace_ReleasesAllServerRefs(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	ctx := context.Background()

	require.NoError(t, e.DidOpen(ctx, ws, tree, goF, "go", "package main"))
	cb := fake.diagCallback()
	require.NotNil(t, cb)
	cb(domlsp.DiagnosticsEvent{WsID: ws, Diagnostics: []domlsp.Diagnostic{{Message: "boom"}}})
	require.Len(t, e.DiagnosticsSnapshot(ws), 1)

	require.Equal(t, 0, fake.closeCount())

	e.ReleaseWorkspace(ctx, ws)
	assert.Equal(t, 1, fake.closeCount(), "ReleaseWorkspace tears the server down despite the held ref")
	assert.Empty(t, e.DiagnosticsSnapshot(ws), "OnReleaseEmpty evicts the snapshot")
}

func TestReleaseWorkspace_UnknownWorkspaceIsNoOp(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)
	e.ReleaseWorkspace(context.Background(), "ghost")
	assert.Equal(t, 0, fake.closeCount())
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

func TestSpawnProcess_BadCommandErrors(t *testing.T) {
	_, err := spawnProcess(
		context.Background(),
		registry.ServerSpec{Command: "crowbar-no-such-lsp-binary-xyz"},
		t.TempDir(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: spawn")
}

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

func as[T any](t *testing.T, v any) T {
	t.Helper()
	got, ok := v.(T)
	require.Truef(t, ok, "got %T, want %T", v, got)
	return got
}
