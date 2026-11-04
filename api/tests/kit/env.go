//go:build integration

package kit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Env is the full integration environment: the real adapter+app+api stack over a Unix socket
// (the production transport) on real SQLite under an isolated home, plus the app and engine
// containers. All WS helpers use deadline-based reads.
type Env struct {
	// URL is the fixed pseudo base URL of the test server ("http://crowbar"). The
	// host is a placeholder: every request is routed to the Unix socket by the
	// env's client/dialer, never by DNS/TCP.
	URL string
	// app exposes the application layer for direct usecase and repository calls.
	app *app.Container
	// engine exposes the engine layer for direct engine calls.
	engine   *engine.Container
	homeDir  string
	adapters *adapter.Container
	session  *migrate.Session

	// server + listener serve the router over socketPath; client + dialer reach it. closeOnce
	// makes exactly one teardown body run, whichever Close variant or cleanup fires first.
	server     *http.Server
	listener   net.Listener
	socketPath string
	client     *http.Client
	dialer     *websocket.Dialer
	closeOnce  sync.Once
	// quiesceMu orders the kit's own writes (read side) against Quiesce
	// (write side); see Quiesce.
	quiesceMu sync.RWMutex
}

// BuildEnv spins up the full server stack (engine → adapter → app → api),
// serves it over a Unix domain socket, and registers graceful cleanup.
// Each call returns a fully isolated environment backed by its own temp home.
func BuildEnv(
	t *testing.T,
) *Env {
	t.Helper()
	return BuildEnvAt(t, tempHome(t))
}

// TempHomeForTest creates an isolated home directory with the same tolerant
// teardown semantics as BuildEnv uses internally. Call this when you need a
// homeDir to pass to BuildEnvAt for crash-recovery / restart tests.
func TempHomeForTest(t *testing.T) string {
	t.Helper()
	return tempHome(t)
}

// tempHome makes an isolated home with a TOLERANT teardown, not t.TempDir(): detached async work
// can write a storages file just after the adapter closes, racing RemoveAll. That race is benign,
// so removal is best-effort and a cleanup error never fails the test.
func tempHome(
	t *testing.T,
) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kitenv-")
	require.NoError(t, err, "kit: create temp home")
	t.Cleanup(func() {
		var err error
		for i := 0; i < 50; i++ {
			if err = os.RemoveAll(dir); err == nil {
				return
			}
			runtime.Gosched()
		}
		// Best-effort: a residual handle from a detached write may still hold a
		// file; the OS reaps the temp tree. Do not fail the test on cleanup.
		t.Logf("kit: remove temp home: %v", err)
	})
	return dir
}

// BuildEnvAt spins up the full stack on a caller-supplied homeDir, so a second call can read
// the first's SQLite rows (crash-recovery tests). Its graceful-Close cleanup is idempotent.
func BuildEnvAt(
	t *testing.T,
	homeDir string,
) *Env {
	t.Helper()
	env, err := NewEnvWithHome(homeDir)
	require.NoError(t, err, "kit: build env")
	// Registered after tempHome's RemoveAll so it runs BEFORE it (LIFO); closeOnce makes it a
	// no-op after an explicit Close variant.
	t.Cleanup(func() { env.Close(t) })
	return env
}

// sockCounter uniquifies per-Env socket paths within a process so a restart
// (a second Env over the same home) binds its OWN short socket rather than
// contending on the first env's path.
var sockCounter atomic.Uint64

// kitSocketPath returns a short unique socket path in the OS temp dir: macOS caps sun_path
// at 104 bytes, so it must not live under the long temp home.
func kitSocketPath() string {
	return filepath.Join(
		os.TempDir(),
		fmt.Sprintf("cbk-%d-%d.sock", os.Getpid(), sockCounter.Add(1)),
	)
}

// NewEnvWithHome is the restart primitive: it serves the real stack rooted at homeDir and
// registers no cleanup (the caller closes). A second call on the same homeDir is a restart.
func NewEnvWithHome(
	homeDir string,
) (*Env, error) {
	ctx := context.Background()
	session, err := BootHome(ctx, homeDir)
	if err != nil {
		return nil, releaseHome(session, err)
	}

	eng, err := engine.New(ctx, engine.WithHomeDir(homeDir))
	if err != nil {
		return nil, releaseHome(session, fmt.Errorf("kit: engine: %w", err))
	}

	adapters, err := adapter.New(adapter.WithHomeDir(homeDir))
	if err != nil {
		eng.Close()
		return nil, releaseHome(session, fmt.Errorf("kit: adapter: %w", err))
	}

	appContainer, err := app.New(ctx, eng, adapters, session, v0.EncodeEntity, v0.NewSidebar)
	if err != nil {
		eng.Close()
		return nil, releaseHome(session, errors.Join(fmt.Errorf("kit: app: %w", err), adapters.Close()))
	}

	router := gin.New()
	apiContainer := v0.New(appContainer, eng, "")
	apiContainer.Register(router.Group("/v0"))

	socketPath := kitSocketPath()
	listener, err := transports.NewSocket("unix://" + socketPath)
	if err != nil {
		appContainer.Close()
		eng.Close()
		return nil, releaseHome(session, errors.Join(fmt.Errorf("kit: gateway: %w", err), adapters.Close()))
	}

	server := &http.Server{Handler: router, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()

	// Both the HTTP client and the WS dialer are host-agnostic: NetDial/DialContext
	// send every connection to the Unix socket regardless of the URL host, so the
	// existing GET/POST/DialX helpers work unchanged over the socket transport.
	dialUnix := func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}
	client := &http.Client{Transport: &http.Transport{DialContext: dialUnix}}
	dialer := &websocket.Dialer{
		NetDial:          func(_, _ string) (net.Conn, error) { return net.Dial("unix", socketPath) },
		HandshakeTimeout: 10 * time.Second,
	}

	return &Env{
		URL:        "http://crowbar",
		app:        appContainer,
		engine:     eng,
		homeDir:    homeDir,
		adapters:   adapters,
		session:    session,
		server:     server,
		listener:   listener,
		socketPath: socketPath,
		client:     client,
		dialer:     dialer,
	}, nil
}

// Close shuts down GRACEFULLY in the daemon's drain order: HTTP, reactors and asynx singletons,
// realtime and engine children, then checkpoint and close the DBs so a second Env can reopen
// the home. Runs at most once (closeOnce).
func (e *Env) Close(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.server.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.Close: server shutdown: %v", err)
		}
		if err := e.app.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.Close: app drain: %v", err)
		}
		e.app.Close()
		e.engine.Close()
		e.releaseStores(t, "Close")
		e.closeListener(t, "Close")
	})
}

// CloseCrashing simulates SIGKILL: no server or app Shutdown, so reactors and asynx work are
// abandoned mid-flight, but the DBs and OS handles close so a second Env can drive recovery.
// Shares closeOnce with the other variants (call exactly one).
func (e *Env) CloseCrashing(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		e.closeAbruptly(t) // drop the listener + active conns, no drain.
		e.app.Close()      // release realtime FDs (watchers/LSP) — not a data drain.
		e.engine.Close()
		e.releaseStores(t, "CloseCrashing")
		e.closeListener(t, "CloseCrashing")
	})
}

// CloseWithoutKilling tears down gracefully but LEAVES the engine's PTY shells and LSP servers
// running, for tests on processes surviving a daemon teardown. Shares closeOnce (call one).
func (e *Env) CloseWithoutKilling(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.server.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.CloseWithoutKilling: server shutdown: %v", err)
		}
		if err := e.app.Shutdown(ctx); err != nil {
			t.Logf("kit.Env.CloseWithoutKilling: app drain: %v", err)
		}
		e.app.Close()
		// Deliberately NOT e.engine.Close(): leave PTY/LSP children running.
		e.releaseStores(t, "CloseWithoutKilling")
		e.closeListener(t, "CloseWithoutKilling")
	})
}

// SocketPath returns the Unix domain socket path this Env serves on.
func (e *Env) SocketPath() string { return e.socketPath }

// HomeDir returns the home directory path used by this Env.
func (e *Env) HomeDir() string { return e.homeDir }

// ShutdownTerminal persists every live session as "suspended"; call it before Close when
// simulating a restart so the next BuildEnvAt can restore them.
func (e *Env) ShutdownTerminal() {
	if e.engine != nil && e.engine.Terminal != nil {
		e.engine.Terminal.Shutdown()
	}
}

// Quiesce blocks until every asynx projection drained: the read-your-writes barrier, since the
// store read model can trail the WS frame. It holds quiesceMu exclusively because the barrier
// refuses commands dispatched while it waits; concurrent kit writes finish first or wait.
func (e *Env) Quiesce() {
	e.quiesceMu.Lock()
	defer e.quiesceMu.Unlock()
	e.app.Repositories.WaitQuiescent()
}

// QuiesceReactors is Quiesce plus the join of every post-commit reactor, which detaches into its
// own goroutine (so Quiesce sees it done at spawn); it is the barrier for cascades like DELETE.
func (e *Env) QuiesceReactors() {
	e.app.Repositories.QuiesceReactors(context.Background())
}

// HoldReactors parks every reactor admitted from now on at the drain gate, so a test can crash
// with a purge deterministically pending. Nothing releases it; crash next.
func (e *Env) HoldReactors() {
	e.app.Repositories.Drain().Gate.Hold()
}

// Now returns a stable deterministic time for test commands.
func Now() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}
