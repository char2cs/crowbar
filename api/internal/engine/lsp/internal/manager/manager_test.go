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
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

// foundLookPath is a WithLookPath stub that reports every command as installed,
// so tests exercise the spawn path without depending on real binaries on PATH.
func foundLookPath() Option {
	return WithLookPath(func(command string) (string, error) {
		return "/usr/bin/" + command, nil
	})
}

// fakeServer is a minimal server.Server stub that tracks calls for assertions.
type fakeServer struct {
	mu      sync.Mutex
	closedN int
	diagFn  func(domlsp.DiagnosticsEvent)
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

func (f *fakeServer) OnDiagnostics(
	fn func(domlsp.DiagnosticsEvent),
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.diagFn = fn
}

func (f *fakeServer) OpenDocs() *server.OpenDocs {
	return server.NewOpenDocs()
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

func (f *fakeServer) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closedN
}

// buildManager constructs a Manager with a fake spawn function that counts
// spawns and produces fakeServers. It uses a real registry with defaults so
// .go maps to the "go" languageID.
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

// --- Tests ---

// TestServerForFile_SpawnOnce verifies that two calls for the same
// (wsID, extension) produce exactly one spawn.
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

// TestServerForFile_TwoLanguages verifies that two languages in one workspace
// each spawn their own server.
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

// TestServerForFile_TwoWorkspaces verifies that the same language in two
// different workspaces each spawn their own server.
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

// TestRefcount_ServerClosedOnLastRelease verifies that Release decrements the
// refcount and calls Close exactly once when it reaches zero.
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

	// refs are now 3 (1 from ServerForFile + 2 from Acquire)
	m.Release(ctx, "ws1", "go")
	m.Release(ctx, "ws1", "go")

	fake := getLastFake()
	require.NotNil(t, fake)
	assert.Equal(t, 0, fake.closeCount(), "server must stay alive with refs > 0")

	m.Release(ctx, "ws1", "go")
	assert.Equal(t, 1, fake.closeCount(), "server must be closed on last release")
}

// TestNoRegistrySpec_ReturnsErrNoServer verifies that an unknown extension
// returns ErrNoServer and does not spawn.
func TestNoRegistrySpec_ReturnsErrNoServer(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "file.unknownext")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoServer))
	assert.Equal(t, int32(0), count.Load(), "must not spawn for unknown extension")
}

// TestServerForFile_BinaryNotInstalled verifies that a registered language
// whose server binary is absent from PATH yields ErrNoServer (graceful
// absence) and never spawns.
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

// TestSpawnExecNotFound_MapsToErrNoServer verifies that a spawn returning
// exec.ErrNotFound (e.g. a half-checked binary) is mapped to ErrNoServer.
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

// TestOnReleaseEmpty_FiresOnLastServerForWorkspace verifies the callback fires
// exactly when a workspace's last server is released, and not while another
// language server for the same workspace is still alive.
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

// TestShutdown_ClosesAllServers verifies that Shutdown closes every live server.
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

// TestOnDiagnostics_FanIn verifies that diagnostics from any spawned server
// are forwarded to the manager-level callback.
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

// TestOnDiagnostics_StampsWsID verifies that an event emitted by a server with
// an empty WsID is stamped with the wsID the server was spawned under before it
// reaches the manager-level callback.
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

	// Servers emit events with an empty WsID; the manager must stamp them.
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

// TestRace_ConcurrentAcquireRelease stress-tests the map + refcount under the
// race detector.
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

// TestRelease_NilEntryIsNoOp verifies that Release for an unknown entry is
// a no-op and does not panic.
func TestRelease_NilEntryIsNoOp(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)
	ctx := context.Background()

	m.Release(ctx, "ws-ghost", "go")
	assert.Equal(t, int32(0), count.Load())
}

// TestAcquire_ExistingEntry verifies that Acquire on a live entry increments
// the refcount so a subsequent Release does not close the server prematurely.
func TestAcquire_ExistingEntry(
	t *testing.T,
) {
	var count atomic.Int32
	m, getLastFake := buildManager(t, &count)
	ctx := context.Background()

	_, err := m.ServerForFile(ctx, "ws1", "/repo", "main.go")
	require.NoError(t, err)

	// Acquire bumps refs to 2.
	m.Acquire("ws1", "go")

	// First release: server must still be alive.
	m.Release(ctx, "ws1", "go")
	fake := getLastFake()
	require.NotNil(t, fake)
	assert.Equal(t, 0, fake.closeCount(), "server must survive first release")

	// Second release: server must be closed.
	m.Release(ctx, "ws1", "go")
	assert.Equal(t, 1, fake.closeCount(), "server must close on last release")
}

// TestSpawnError verifies that a spawn failure is propagated as an error.
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

// TestOnDiagnostics_WiredBeforeSpawn verifies that setting the manager-level
// diagnostics callback before any spawns wires it into subsequently spawned
// servers.
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

// TestGetOrSpawn_RaceGap exercises the path where two goroutines race to
// spawn the same (wsID, languageID) simultaneously; exactly one server must
// be kept alive.
func TestGetOrSpawn_RaceGap(
	t *testing.T,
) {
	var count atomic.Int32
	var closedN atomic.Int32

	// Use a gate channel so both goroutines are past the first lock check
	// before either proceeds to spawn — this maximises the chance of hitting
	// the double-spawn race-gap path.
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

	// Release both goroutines simultaneously.
	close(gate)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.NotNil(t, srvs[0])
	require.NotNil(t, srvs[1])

	// One of the two spawned servers was discarded (closed immediately).
	spawnedCount := count.Load()
	if spawnedCount == 2 {
		assert.Equal(t, int32(1), closedN.Load(), "loser server must be closed on race-gap")
	} else {
		assert.Equal(t, int32(1), spawnedCount)
	}
}

// TestGetOrSpawn_RaceGapForced uses the internal manager directly to force the
// race-gap re-check branch: insert an entry while spawn is in flight.
func TestGetOrSpawn_RaceGapForced(
	t *testing.T,
) {
	var closedN atomic.Int32
	winner := &countingServer{fakeServer: &fakeServer{}, closedN: &closedN}
	loser := &countingServer{fakeServer: &fakeServer{}, closedN: &closedN}

	// spawnCalled synchronises the test: first call blocks, test inserts the
	// winning entry, then unblocks the loser.
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

	// Pre-seed: cause the first ServerForFile to block inside spawn while we
	// manually insert the winning entry to force the re-check path.
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		_, err := m.ServerForFile(ctx, "wsR", "/r", "main.go")
		done <- err
	}()

	// Wait until spawn is called, then directly insert the winner via a
	// second ServerForFile on a fresh goroutine that completes before the
	// loser unblocks.
	<-spawnCalled

	// Spawn the winner synchronously via a second call — this one must not
	// block because we use a separate spawn counter path.
	// We reach into the impl via a cast to inject directly.
	impl := m.(*manager)
	impl.mu.Lock()
	impl.pool["wsR|go"] = &entry{srv: winner, refs: 1}
	impl.mu.Unlock()

	// Unblock the loser's spawn — it will find the winner in the map and
	// close itself.
	close(unblock)

	err := <-done
	require.NoError(t, err)
	assert.Equal(t, int32(1), closedN.Load(), "loser must be closed when winner already in map")
}

// TestAcquire_MissingEntry verifies that Acquire on an unknown (wsID,
// languageID) pair is a no-op and does not panic.
func TestAcquire_MissingEntry(
	t *testing.T,
) {
	var count atomic.Int32
	m, _ := buildManager(t, &count)

	// Calling Acquire for a key that was never spawned must not panic.
	m.Acquire("no-such-ws", "go")
	assert.Equal(t, int32(0), count.Load())
}

// countingServer wraps fakeServer and counts Close calls via an atomic.
type countingServer struct {
	*fakeServer
	closedN *atomic.Int32
}

func (c *countingServer) Close() error {
	c.closedN.Add(1)
	return c.fakeServer.Close()
}
