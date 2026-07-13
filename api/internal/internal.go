package internal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/selfinstall"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container owning the HTTP server, its listener, and the
// adapter layer (closed on shutdown).
type Container struct {
	server   *http.Server
	listener net.Listener
	engines  *engine.Container
	adapter  *adapter.Container
	app      *app.Container
	api      *crowbarapi.Container
}

type rootOpts struct {
	homeDir string
}

// Option configures internal.New.
type Option func(*rootOpts)

// WithHomeDir roots all on-disk state under dir (test isolation).
func WithHomeDir(
	dir string,
) Option {
	return func(o *rootOpts) {
		o.homeDir = dir
	}
}

// New wires engine → adapter → app → api in order and returns the root container.
func New(
	ctx context.Context,
	host string,
	staticFS fs.FS,
	options ...Option,
) (*Container, error) {
	cfg := rootOpts{}
	for _, o := range options {
		o(&cfg)
	}

	listener, err := gateway.New(host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}

	engines, err := engine.New(ctx, engine.WithHomeDir(cfg.homeDir))
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New(adapterOptions(cfg.homeDir)...)
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	appContainer, err := app.New(ctx, engines, adapters)
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	apiContainer, err := crowbarapi.New(appContainer, engines, staticFS)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	// Install the crowbar binary into <home>/bin so vendor CLI hooks can invoke
	// `crowbar hook` by absolute path. Use the container's CONFIGURED home so tests
	// and dev instances (WithHomeDir) stay isolated; fall back to the resolved global
	// home only when none was configured — the production `serve` path. Best-effort:
	// never block startup, and never write to the real ~/.crowbar from a test.
	installHome := cfg.homeDir
	if installHome == "" {
		installHome = metadata.GetHomePath()
	}
	if p, err := selfinstall.Install(installHome); err == nil {
		_ = p // path is re-derived by the agent engine when rendering descriptors
	}

	return &Container{
		server:   &http.Server{Handler: apiContainer.Handler(), ReadHeaderTimeout: 30 * time.Second},
		listener: listener,
		engines:  engines,
		adapter:  adapters,
		app:      appContainer,
		api:      apiContainer,
	}, nil
}

func adapterOptions(
	homeDir string,
) []adapter.Option {
	if homeDir == "" {
		return nil
	}
	return []adapter.Option{adapter.WithHomeDir(homeDir)}
}

// Run serves until ctx is cancelled, then gracefully shuts down. It also watches
// for an early Serve failure: if Serve returns before shutdown is requested
// (listener invalidated, accept failing permanently under FD pressure), the
// error is surfaced so the process exits non-zero and its supervisor restarts
// the daemon — rather than silently hanging "up" while accepting no connections.
func (c *Container) Run(
	ctx context.Context,
) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- c.server.Serve(c.listener) }()
	select {
	case <-ctx.Done():
		// Ordered, bounded graceful shutdown (spec §3.8): under one ~5s deadline,
		// stop accepting new requests + drain in-flight HTTP, then hand off to
		// app.Shutdown, which quiesces every writer in dependency order — the PTY
		// reap path first (killing a vendor CLI fires the exit callback that Exits
		// its runner and closes the turn it abandoned), then the post-commit
		// reactors, then each asynx singleton. All of it happens BEFORE the deferred
		// Close() WAL-checkpoints and closes the DBs (adapter.Close), so nothing is
		// mid-write when the DBs shut. Every wait honors shutdownCtx.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpErr := c.server.Shutdown(shutdownCtx)
		drainErr := c.app.Shutdown(shutdownCtx)
		return errors.Join(httpErr, drainErr)
	case err := <-serveErr:
		// ErrServerClosed only occurs after Shutdown (handled above), so any error
		// arriving here is a genuine listen/accept failure.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("internal: serve: %w", err)
		}
		return nil
	}
}

// Close tears the backend down in dependency order: the HTTP server has already
// stopped accepting connections (Run shuts it down before Close runs), so it
// first closes the app layer's lazy realtime resource lifecycles (file watchers
// and LSP hosts, which http.Server.Shutdown leaves running on hijacked WebSocket
// connections), then releases the adapter layer and the listener.
func (c *Container) Close() {
	c.app.Close()
	// Tear down engine OS resources (live PTY child processes + master FDs) so a
	// shutdown/restart doesn't orphan every open terminal's shell and the dev
	// servers it spawned. On the graceful path the terminals are ALREADY quiesced
	// (app.Shutdown step 1, which had to run before the aggregates drained), so this
	// only finishes the job for the paths that never reached it — a Serve failure, or
	// a caller that closes without running. It never re-enters that join.
	c.engines.Close()
	// The DBs go LAST, once every writer above is quiesced: the exit callbacks fired
	// by the PTY teardown write runner Exits and turn closes, and closing the store
	// out from under them is what used to leave a chat spinning forever.
	_ = c.adapter.Close()
	_ = c.listener.Close()
}
