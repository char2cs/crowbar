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
	"github.com/char2cs/crowbar/api/internal/testutil/accessfake"
)

type Env struct {
	URL      string
	app      *app.Container
	engine   *engine.Container
	homeDir  string
	adapters *adapter.Container
	session  *migrate.Session

	server     *http.Server
	listener   net.Listener
	socketPath string
	client     *http.Client
	dialer     *websocket.Dialer
	closeOnce  sync.Once
	quiesceMu  sync.RWMutex
}

func BuildEnv(
	t *testing.T,
) *Env {
	t.Helper()
	return BuildEnvAt(t, tempHome(t))
}

func TempHomeForTest(t *testing.T) string {
	t.Helper()
	return tempHome(t)
}

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
		t.Logf("kit: remove temp home: %v", err)
	})
	return dir
}

func BuildEnvAt(
	t *testing.T,
	homeDir string,
) *Env {
	t.Helper()
	env, err := NewEnvWithHome(homeDir)
	require.NoError(t, err, "kit: build env")
	t.Cleanup(func() { env.Close(t) })
	return env
}

var sockCounter atomic.Uint64

func kitSocketPath() string {
	return filepath.Join(
		os.TempDir(),
		fmt.Sprintf("cbk-%d-%d.sock", os.Getpid(), sockCounter.Add(1)),
	)
}

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
	apiContainer := v0.New(appContainer, eng, accessfake.Closed())
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

func (e *Env) CloseCrashing(
	t *testing.T,
) {
	t.Helper()
	e.closeOnce.Do(func() {
		e.closeAbruptly(t)
		e.app.Close()
		e.engine.Close()
		e.releaseStores(t, "CloseCrashing")
		e.closeListener(t, "CloseCrashing")
	})
}

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
		e.releaseStores(t, "CloseWithoutKilling")
		e.closeListener(t, "CloseWithoutKilling")
	})
}

func (e *Env) SocketPath() string { return e.socketPath }

func (e *Env) HomeDir() string { return e.homeDir }

func (e *Env) ShutdownTerminal() {
	if e.engine != nil && e.engine.Terminal != nil {
		e.engine.Terminal.Shutdown()
	}
}

func (e *Env) Quiesce() {
	e.quiesceMu.Lock()
	defer e.quiesceMu.Unlock()
	e.app.Repositories.WaitQuiescent()
}

func (e *Env) QuiesceReactors() {
	e.app.Repositories.QuiesceReactors(context.Background())
}

func (e *Env) HoldReactors() {
	e.app.Repositories.Drain().Gate.Hold()
}

func Now() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}
