package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

func foundLookPath() Option {
	return WithLookPath(func(command string) (string, error) {
		return "/usr/bin/" + command, nil
	})
}

type fakeServer struct {
	mu      sync.Mutex
	closedN int
	diagFn  func(domlsp.DiagnosticsEvent)
	exitFn  func()
}

func (f *fakeServer) Request(
	_ context.Context,
	_ string,
	_ any,
) (json.RawMessage, error) {
	return nil, nil
}

func (f *fakeServer) Notify(
	_ context.Context,
	_ string,
	_ any,
) error {
	return nil
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

func (f *fakeServer) fireExit() {
	f.mu.Lock()
	fn := f.exitFn
	f.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (f *fakeServer) OpenDocs() *server.OpenDocs {
	return server.NewOpenDocs()
}

func (f *fakeServer) Replay(
	_ context.Context,
) error {
	return nil
}

func (f *fakeServer) SemanticTokens() semtok.Support {
	return semtok.Support{}
}

func (f *fakeServer) CanExecute(
	_ string,
) bool {
	return false
}

func (f *fakeServer) ExecuteCommand(
	_ context.Context,
	_ any,
) (json.RawMessage, []json.RawMessage, error) {
	return nil, nil, nil
}

func (f *fakeServer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closedN++
	return nil
}

func (f *fakeServer) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closedN
}

func buildManager(
	t *testing.T,
	spawnCount *atomic.Int32,
) (Manager, func() *fakeServer) {
	t.Helper()
	var mu sync.Mutex
	var last *fakeServer

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		spawnCount.Add(1)
		fs := &fakeServer{}
		mu.Lock()
		last = fs
		mu.Unlock()
		return fs, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())

	getLastFake := func() *fakeServer {
		mu.Lock()
		defer mu.Unlock()
		return last
	}

	return m, getLastFake
}

func TestServerForFile_SpawnOnce(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	srv1, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	srv2, err := m.ServerForFile(ctx, "ws1", "/repo", "util.go")
	require.NoError(t, err)

	assert.Equal(t, int32(1), count.Load(), "expected exactly one spawn")
	assert.Same(t, srv1, srv2, "expected the same server instance")
}

func TestServerForFile_TwoLanguages(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	srvGo, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	srvTS, err := m.ServerForFile(ctx, "ws1", "/repo", "app.ts")
	require.NoError(t, err)

	assert.Equal(t, int32(2), count.Load(), "expected two spawns for two languages")
	assert.NotSame(t, srvGo, srvTS, "expected distinct server instances")
}

func TestServerForFile_TwoWorkspaces(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	srv1, err := m.ServerForFile(ctx, "ws1", "/repo1", "main.go")
	require.NoError(t, err)

	srv2, err := m.ServerForFile(ctx, "ws2", "/repo2", "main.go")
	require.NoError(t, err)

	assert.Equal(t, int32(2), count.Load(), "expected two spawns for two workspaces")
	assert.NotSame(t, srv1, srv2, "expected distinct server instances")
}

func TestRefcount_ServerClosedOnLastRelease(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	m.Acquire("ws1", "go")
	m.Acquire("ws1", "go")

	m.Release(ctx, "ws1", "go")
	m.Release(ctx, "ws1", "go")

	fake := getLastFake()
	require.NotNil(t, fake)
	assert.Equal(t, 0, fake.closeCount(), "server must stay alive with refs > 0")

	m.Release(ctx, "ws1", "go")
	assert.Equal(t, 1, fake.closeCount(), "server must be closed on last release")
}

func TestRunningServerForFile_ReturnsWithoutTouchingRefcount(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	srv, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	got, err := m.RunningServerForFile("ws1", "main.go")
	require.NoError(t, err)
	assert.Same(t, srv, got, "must return the already-running server")
	assert.Equal(t, int32(1), count.Load(), "lookup must not spawn")

	fake := getLastFake()
	require.NotNil(t, fake)
	m.Release(ctx, "ws1", "go")
	assert.Equal(t, 1, fake.closeCount(), "lookup did not add a ref, so one Release tears down")
}

func TestRunningServerForFile_NoSpecOrServerErrNoServer(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)

	_, err := m.RunningServerForFile("ws1", "file.unknownext")
	require.ErrorIs(t, err, ErrNoServer)

	_, err = m.RunningServerForFile("ws1", "main.go")
	require.ErrorIs(t, err, ErrNoServer)
	assert.Equal(t, int32(0), count.Load(), "lookup must never spawn")
}

func TestNoRegistrySpec_ReturnsErrNoServer(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "file.unknownext")
	require.ErrorIs(t, err, ErrNoServer)
	assert.Equal(t, int32(0), count.Load(), "must not spawn for unknown extension")
}

func TestServerForFile_BinaryNotInstalled(
	t *testing.T,
) {
	var count atomic.Int32
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		count.Add(1)
		return &fakeServer{}, nil
	}
	reg := registry.New(nil)
	m := New(reg, spawn, WithLookPath(func(string) (string, error) {
		return "", exec.ErrNotFound
	}))

	_, err := m.ServerForFile(context.Background(), "ws1", "/repo", "main.go")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoServer)
	assert.Equal(t, int32(0), count.Load(), "must not spawn when binary is absent")
}

func TestSpawnExecNotFound_MapsToErrNoServer(
	t *testing.T,
) {
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return nil, exec.ErrNotFound
	}
	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())

	_, err := m.ServerForFile(context.Background(), "ws1", "/repo", "main.go")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoServer)
}

func TestOnReleaseEmpty_FiresOnLastServerForWorkspace(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	var released []string
	var mu sync.Mutex
	m.OnReleaseEmpty(func(wsID string) {
		mu.Lock()
		released = append(released, wsID)
		mu.Unlock()
	})

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	_, err = m.ServerForFile(ctx, "ws1", "/repo", "app.ts")
	require.NoError(t, err)

	m.Release(ctx, "ws1", "go")
	mu.Lock()
	assert.Empty(t, released, "must not fire while ts server is still alive")
	mu.Unlock()

	m.Release(ctx, "ws1", "typescript")
	mu.Lock()
	assert.Equal(t, []string{"ws1"}, released, "must fire when last server released")
	mu.Unlock()
}

func TestShutdown_ClosesAllServers(
	t *testing.T,
) {
	var count atomic.Int32
	var mu sync.Mutex
	var fakes []*fakeServer

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		count.Add(1)
		fs := &fakeServer{}
		mu.Lock()
		fakes = append(fakes, fs)
		mu.Unlock()
		return fs, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	_, err = m.ServerForFile(ctx, "ws1", "/repo", "app.ts")
	require.NoError(t, err)

	_, err = m.ServerForFile(ctx, "ws2", "/repo2", "main.go")
	require.NoError(t, err)

	m.Shutdown(ctx)

	mu.Lock()
	defer mu.Unlock()
	for i, fs := range fakes {
		assert.Equal(t, 1, fs.closeCount(), "fake[%d] must be closed by Shutdown", i)
	}
}

func TestOnDiagnostics_FanIn(
	t *testing.T,
) {
	var count atomic.Int32
	var mu sync.Mutex
	var fakes []*fakeServer

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		count.Add(1)
		fs := &fakeServer{}
		mu.Lock()
		fakes = append(fakes, fs)
		mu.Unlock()
		return fs, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())

	var received []domlsp.DiagnosticsEvent
	var diagMu sync.Mutex
	m.OnDiagnostics(func(
		e domlsp.DiagnosticsEvent,
	) {
		diagMu.Lock()
		received = append(received, e)
		diagMu.Unlock()
	})

	ctx := context.Background()
	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	_, err = m.ServerForFile(ctx, "ws1", "/repo", "app.ts")
	require.NoError(t, err)

	mu.Lock()
	localFakes := append([]*fakeServer(nil), fakes...)
	mu.Unlock()

	require.Len(t, localFakes, 2)

	event1 := domlsp.DiagnosticsEvent{WsID: "ws1", Diagnostics: []domlsp.Diagnostic{{Message: "err1"}}}
	event2 := domlsp.DiagnosticsEvent{WsID: "ws1", Diagnostics: []domlsp.Diagnostic{{Message: "err2"}}}

	localFakes[0].mu.Lock()
	fn0 := localFakes[0].diagFn
	localFakes[0].mu.Unlock()
	require.NotNil(t, fn0, "OnDiagnostics must have been wired on the first server")

	localFakes[1].mu.Lock()
	fn1 := localFakes[1].diagFn
	localFakes[1].mu.Unlock()
	require.NotNil(t, fn1, "OnDiagnostics must have been wired on the second server")

	fn0(event1)
	fn1(event2)

	diagMu.Lock()
	defer diagMu.Unlock()
	assert.Len(t, received, 2, "both events must reach the manager-level callback")
}

func TestOnDiagnostics_StampsWsID(
	t *testing.T,
) {
	var mu sync.Mutex
	var fakes []*fakeServer

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		fs := &fakeServer{}
		mu.Lock()
		fakes = append(fakes, fs)
		mu.Unlock()
		return fs, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())

	var received []domlsp.DiagnosticsEvent
	var diagMu sync.Mutex
	m.OnDiagnostics(func(
		e domlsp.DiagnosticsEvent,
	) {
		diagMu.Lock()
		received = append(received, e)
		diagMu.Unlock()
	})

	ctx := context.Background()
	_, err := m.ServerForFile(ctx, "ws-go", "/repo", "main.go")
	require.NoError(t, err)

	_, err = m.ServerForFile(ctx, "ws-ts", "/repo", "app.ts")
	require.NoError(t, err)

	mu.Lock()
	localFakes := append([]*fakeServer(nil), fakes...)
	mu.Unlock()
	require.Len(t, localFakes, 2)

	localFakes[0].mu.Lock()
	fn0 := localFakes[0].diagFn
	localFakes[0].mu.Unlock()
	require.NotNil(t, fn0)

	localFakes[1].mu.Lock()
	fn1 := localFakes[1].diagFn
	localFakes[1].mu.Unlock()
	require.NotNil(t, fn1)

	fn0(domlsp.DiagnosticsEvent{Diagnostics: []domlsp.Diagnostic{{Message: "go"}}})
	fn1(domlsp.DiagnosticsEvent{Diagnostics: []domlsp.Diagnostic{{Message: "ts"}}})

	diagMu.Lock()
	defer diagMu.Unlock()
	require.Len(t, received, 2)
	byWs := map[string]string{}
	for _, e := range received {
		require.Len(t, e.Diagnostics, 1)
		byWs[e.WsID] = e.Diagnostics[0].Message
	}
	assert.Equal(t, "go", byWs["ws-go"])
	assert.Equal(t, "ts", byWs["ws-ts"])
}

func TestRace_ConcurrentAcquireRelease(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	var wg sync.WaitGroup
	const goroutines = 20
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			m.Acquire("ws1", "go")
			m.Release(ctx, "ws1", "go")
		}()
	}
	wg.Wait()
}

func TestRelease_NilEntryIsNoOp(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	m.Release(ctx, "ws-ghost", "go")
	assert.Equal(t, int32(0), count.Load())
}

func TestAcquire_ExistingEntry(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	m.Acquire("ws1", "go")

	m.Release(ctx, "ws1", "go")
	fake := getLastFake()
	require.NotNil(t, fake)
	assert.Equal(t, 0, fake.closeCount(), "server must survive first release")

	m.Release(ctx, "ws1", "go")
	assert.Equal(t, 1, fake.closeCount(), "server must close on last release")
}

func TestSpawnError(
	t *testing.T,
) {
	spawnErr := errors.New("spawn failed")
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return nil, spawnErr
	}
	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.Error(t, err)
	assert.ErrorIs(t, err, spawnErr)
}

func TestOnDiagnostics_WiredBeforeSpawn(
	t *testing.T,
) {
	var count atomic.Int32
	var mu sync.Mutex
	var fakes []*fakeServer

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		count.Add(1)
		fs := &fakeServer{}
		mu.Lock()
		fakes = append(fakes, fs)
		mu.Unlock()
		return fs, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())

	var received []domlsp.DiagnosticsEvent
	var diagMu sync.Mutex
	m.OnDiagnostics(func(
		e domlsp.DiagnosticsEvent,
	) {
		diagMu.Lock()
		received = append(received, e)
		diagMu.Unlock()
	})

	ctx := context.Background()
	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	mu.Lock()
	localFakes := append([]*fakeServer(nil), fakes...)
	mu.Unlock()

	require.Len(t, localFakes, 1)
	localFakes[0].mu.Lock()
	fn := localFakes[0].diagFn
	localFakes[0].mu.Unlock()
	require.NotNil(t, fn, "callback must be wired into spawned server")

	event := domlsp.DiagnosticsEvent{WsID: "ws1", Diagnostics: []domlsp.Diagnostic{{Message: "test"}}}
	fn(event)

	diagMu.Lock()
	defer diagMu.Unlock()
	assert.Len(t, received, 1)
}

func TestGetOrSpawn_RaceGap(
	t *testing.T,
) {
	var count atomic.Int32
	var closedN atomic.Int32

	gate := make(chan struct{})

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		<-gate
		count.Add(1)
		fs := &fakeServer{}
		return &countingServer{fakeServer: fs, closedN: &closedN}, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	srvs := make([]server.Server, 2)
	for i := range 2 {
		go func(idx int) {
			defer wg.Done()
			srvs[idx], errs[idx] = m.ServerForFile(ctx, "ws1", "/repo", "main.go")
		}(i)
	}

	close(gate)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.NotNil(t, srvs[0])
	require.NotNil(t, srvs[1])

	spawnedCount := count.Load()
	if spawnedCount == 2 {
		assert.Equal(t, int32(1), closedN.Load(), "loser server must be closed on race-gap")
	} else {
		assert.Equal(t, int32(1), spawnedCount)
	}
}

func TestGetOrSpawn_RaceGapForced(
	t *testing.T,
) {
	var closedN atomic.Int32
	winner := &countingServer{fakeServer: &fakeServer{}, closedN: &closedN}
	loser := &countingServer{fakeServer: &fakeServer{}, closedN: &closedN}

	spawnCalled := make(chan struct{}, 1)
	unblock := make(chan struct{})
	spawnCount := 0

	reg := registry.New(nil)
	m := New(reg, func(
		_ context.Context,
		spec registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		spawnCount++
		if spawnCount == 1 {
			spawnCalled <- struct{}{}
			<-unblock
			return loser, nil
		}
		return winner, nil
	}, foundLookPath())

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		_, err := m.ServerForFile(ctx, "wsR", "/r", "main.go")
		done <- err
	}()

	<-spawnCalled

	impl := m.(*manager)
	impl.mu.Lock()
	impl.pool["wsR|go"] = &entry{srv: winner, refs: 1}
	impl.mu.Unlock()

	close(unblock)

	err := <-done
	require.NoError(t, err)
	assert.Equal(t, int32(1), closedN.Load(), "loser must be closed when winner already in map")
}

func TestAcquire_MissingEntry(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)

	m.Acquire("no-such-ws", "go")
	assert.Equal(t, int32(0), count.Load())
}

func TestOnServerExit_EvictsDeadEntryAndRespawns(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	srv1, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	require.Equal(t, int32(1), count.Load())

	dead := getLastFake()
	require.NotNil(t, dead)

	dead.fireExit()
	assert.Equal(t, 1, dead.closeCount(), "exit must close the dead server (failing in-flight waiters)")

	srv2, err := m.ServerForFile(ctx, "ws1", "/repo", "util.go")
	require.NoError(t, err)
	assert.Equal(t, int32(2), count.Load(), "a fresh server must be spawned after eviction")
	assert.NotSame(t, srv1, srv2, "the corpse must not be handed back")
}

func TestOnServerExit_FiresEmptyCallbackOnLastServer(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	var released []string
	var mu sync.Mutex
	m.OnReleaseEmpty(func(wsID string) {
		mu.Lock()
		released = append(released, wsID)
		mu.Unlock()
	})

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	getLastFake().fireExit()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"ws1"}, released, "evicting the last server fires the empty callback")
}

func TestOnServerExit_UnknownEntryIsNoOp(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	fake := getLastFake()

	fake.fireExit()
	fake.fireExit()
	assert.Equal(t, 1, fake.closeCount(), "second exit is a no-op")
}

func TestReleaseWorkspace_ClosesAllServersForWorkspace(
	t *testing.T,
) {
	var count atomic.Int32
	var mu sync.Mutex
	var fakes []*fakeServer

	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		count.Add(1)
		fs := &fakeServer{}
		mu.Lock()
		fakes = append(fakes, fs)
		mu.Unlock()
		return fs, nil
	}

	reg := registry.New(nil)
	m := New(reg, spawn, foundLookPath())
	ctx := context.Background()

	var released []string
	var relMu sync.Mutex
	m.OnReleaseEmpty(func(wsID string) {
		relMu.Lock()
		released = append(released, wsID)
		relMu.Unlock()
	})

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	m.Acquire("ws1", "go")
	_, err = m.ServerForFile(ctx, "ws1", "/repo", "app.ts")
	require.NoError(t, err)
	_, err = m.ServerForFile(ctx, "ws2", "/repo2", "main.go")
	require.NoError(t, err)

	mu.Lock()
	require.Len(t, fakes, 3)
	ws1Go, ws1TS, ws2Go := fakes[0], fakes[1], fakes[2]
	mu.Unlock()

	m.ReleaseWorkspace(ctx, "ws1")

	assert.Equal(t, 1, ws1Go.closeCount(), "ws1 go server closed despite held refs")
	assert.Equal(t, 1, ws1TS.closeCount(), "ws1 ts server closed despite held refs")
	assert.Equal(t, 0, ws2Go.closeCount(), "ws2 server must be untouched")

	relMu.Lock()
	assert.Equal(t, []string{"ws1"}, released, "empty callback fires once for the released workspace")
	relMu.Unlock()

	_, err = m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	assert.Equal(t, int32(4), count.Load(), "released ws1 respawns; ws2 is unchanged")
}

func TestReleaseWorkspace_UnknownWorkspaceIsNoOp(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)

	fired := false
	m.OnReleaseEmpty(func(string) { fired = true })

	m.ReleaseWorkspace(context.Background(), "ghost")
	assert.False(t, fired, "no servers means no empty callback")
}

type countingServer struct {
	*fakeServer
	closedN *atomic.Int32
}

func (c *countingServer) Close() error {
	c.closedN.Add(1)
	return c.fakeServer.Close()
}

func TestStatus_ReflectsRegistryInstallAndPool(
	t *testing.T,
) {
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return &fakeServer{}, nil
	}
	installed := false
	m := New(registry.New(nil), spawn, WithLookPath(func(command string) (string, error) {
		if !installed {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + command, nil
	}))
	ctx := context.Background()

	assert.Equal(t, domlsp.ServerUnsupported, m.Status("ws1", "README").State)

	st := m.Status("ws1", "main.go")
	assert.Equal(t, domlsp.ServerNotInstalled, st.State)
	assert.Equal(t, "gopls", st.Command)
	assert.Equal(t, "go", st.LanguageID)

	installed = true
	assert.Equal(t, domlsp.ServerStopped, m.Status("ws1", "main.go").State)

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)
	assert.Equal(t, domlsp.ServerRunning, m.Status("ws1", "main.go").State)
	assert.Equal(t, domlsp.ServerStopped, m.Status("ws2", "main.go").State,
		"another workspace's server is not this one's")

	m.Release(ctx, "ws1", "go")
	assert.Equal(t, domlsp.ServerStopped, m.Status("ws1", "main.go").State)
}
