package internal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/core/selfinstall"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container owning the HTTP server, its listener, and the
// adapter layer (closed on shutdown).
type Container struct {
	server   *http.Server
	listener net.Listener
	// session holds the home from before the stores open until after they close.
	session *migrate.Session
	// hooks is the loopback listener for vendor CLIs' own HTTP hooks. It serves
	// the hook route of the same router and nothing else of it.
	hooks   *hookserver.Server
	engines *engine.Container
	adapter *adapter.Container
	app     *app.Container
	api     *crowbarapi.Container
}

type rootOpts struct {
	homeDir string
	logs    logring.Ring
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

// WithLogRing serves the ring's records on GET /v0/console/logs.
func WithLogRing(
	ring logring.Ring,
) Option {
	return func(o *rootOpts) {
		o.logs = ring
	}
}

// New takes the home and runs its pre-open migrations, then wires engine →
// adapter → app → api in order and returns the root container.
func New(
	ctx context.Context,
	host string,
	staticFS fs.FS,
	options ...Option,
) (c *Container, err error) {
	cfg := rootOpts{}
	for _, o := range options {
		o(&cfg)
	}

	listener, err := gateway.New(host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}
	opened := &Container{listener: listener}
	defer func() { //nolint:contextcheck // the Close methods take no ctx: a failed boot's teardown runs even after ctx is done.
		if err != nil {
			err = errors.Join(err, opened.abandon())
		}
	}()

	h, session, err := openHome(ctx, cfg.homeDir)
	if err != nil {
		return nil, fmt.Errorf("internal: migrate: %w", err)
	}
	opened.session = session

	opened.engines, err = engine.New(ctx, engine.WithHomeDir(cfg.homeDir), engine.WithDaemonSocket(socketOf(listener)))
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	opened.adapter, err = adapter.New(adapter.WithHomeDir(h.Root()))
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	opened.app, err = app.New(ctx, opened.engines, opened.adapter, session)
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	//nolint:contextcheck // snapshot providers are context-free by design; this call only threads the log ring option.
	opened.api, err = crowbarapi.New(opened.app, opened.engines, staticFS, v0.WithLogs(cfg.logs))
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	// Install the crowbar binary into <home>/bin so vendor CLI hooks can invoke
	// `crowbar hook` by absolute path, into the home the adapter opened so tests
	// and dev instances stay isolated. Best-effort: never block startup.
	installHome := opened.adapter.CrowbarHome()
	if p, err := selfinstall.Install(installHome); err == nil {
		_ = p // path is re-derived by the agent engine when rendering descriptors
	}

	// Published before anything can spawn a CLI: the spawner reads it to point the
	// CLI's hooks here.
	hooks, err := hookserver.Listen(ctx, opened.api.Handler())
	if err != nil {
		return nil, fmt.Errorf("internal: hooks: %w", err)
	}
	if err := hookserver.Publish(installHome, hooks.URL()); err != nil {
		_ = hooks.Close()
		return nil, fmt.Errorf("internal: hooks: %w", err)
	}
	opened.hooks = hooks
	opened.server = &http.Server{Handler: opened.api.Handler(), ReadHeaderTimeout: 30 * time.Second}
	return opened, nil
}

// abandon closes what a New that failed had opened, in Close's order, so the
// home is released only after every store is closed.
func (c *Container) abandon() error {
	if c.api != nil {
		c.api.Close()
	}
	if c.app != nil {
		c.app.Close()
	}
	if c.engines != nil {
		c.engines.Close()
	}
	var errs []error
	if c.adapter != nil {
		errs = append(errs, c.adapter.Close())
	}
	errs = append(errs, c.listener.Close())
	if c.session != nil {
		errs = append(errs, c.session.Close())
	}
	return errors.Join(errs...)
}

// socketOf is the unix socket listener is bound to, or "" for any other kind.
func socketOf(
	listener net.Listener,
) string {
	if addr, ok := listener.Addr().(*net.UnixAddr); ok {
		return addr.Name
	}
	return ""
}

// Socket is the unix socket the daemon bound, empty when it listens on TCP.
func (c *Container) Socket() string {
	return c.engines.DaemonSocket
}

// openHome takes the home rooted at dir (the default home when dir is empty)
// and runs the migrations that must finish before any store opens. Until the
// daemon can serve a halt, a halted boot is an error and releases the home.
func openHome(
	ctx context.Context,
	dir string,
) (home.Home, *migrate.Session, error) {
	h, err := home.ResolveDir(dir)
	if err != nil {
		return home.Home{}, nil, err
	}
	session, err := migrations.Begin(ctx, h, migrate.Options{Version: metadata.GetVersion()})
	if err != nil {
		return home.Home{}, nil, err
	}
	if err := session.RunPreOpen(ctx, migrations.PreOpenImpls()); err != nil {
		return home.Home{}, nil, errors.Join(err, session.Close())
	}
	return h, session, nil
}

// Home is the Crowbar home the daemon opened.
func (c *Container) Home() string {
	return c.adapter.CrowbarHome()
}

// Run serves until ctx is cancelled, then gracefully shuts down. It also watches
// for an early Serve failure: if Serve returns before shutdown is requested
// (listener invalidated, accept failing permanently under FD pressure), the
// error is surfaced so the process exits non-zero and its supervisor restarts
// the daemon — rather than silently hanging "up" while accepting no connections.
func (c *Container) Run(
	ctx context.Context,
) error {
	serveErr := make(chan error, 2)
	go func() { serveErr <- c.server.Serve(c.listener) }()
	go func() { serveErr <- c.hooks.Serve() }()
	select {
	case <-ctx.Done():
		// Ordered, bounded graceful shutdown (spec §3.8): stop accepting new requests +
		// drain in-flight HTTP, then hand off to app.Shutdown, which quiesces every writer
		// in dependency order — the PTY reap path first (killing a vendor CLI fires the exit
		// callback that Exits its runner and closes the turn it abandoned), then the
		// post-commit reactors, then each asynx singleton. All of it happens BEFORE the
		// deferred Close() WAL-checkpoints and closes the DBs (adapter.Close), so nothing is
		// mid-write when the DBs shut.
		//
		// THE TWO PHASES GET SEPARATE DEADLINES ON PURPOSE, and this is a bug fix, not a
		// nicety. They used to share one ~5s budget, which let the LEAST important phase
		// starve the MOST important one: a single slow in-flight request — an un-timeout'd
		// `git fetch` is the known case — kept server.Shutdown busy until the whole budget
		// lapsed, so app.Shutdown then ran with an already-dead context, the terminal
		// quiesce returned instantly on <-ctx.Done() WITHOUT recording the CLI deaths it
		// exists to record, and the "chat spins forever across restarts" bug was back.
		//
		// Draining in-flight HTTP is best-effort — an abandoned request is retried by the
		// client — so it gets a short, capped slice and IS allowed to be cut off. The writer
		// quiesce is where durable state is written, so it gets its own fresh budget that no
		// HTTP request can touch. The desktop supervisor SIGKILLs the daemon 3s after
		// SIGTERM (desktop/src-tauri/src/lib.rs), so httpDrainGrace is deliberately small to
		// leave the writers the majority of that window; writerDrainGrace only binds when
		// the daemon runs with no supervisor (headless / tests), where a wedged reaper must
		// still not hang the process.
		const (
			httpDrainGrace   = 1 * time.Second
			writerDrainGrace = 4 * time.Second
		)
		httpCtx, cancelHTTP := context.WithTimeout(context.WithoutCancel(ctx), httpDrainGrace)
		defer cancelHTTP()
		httpErr := errors.Join(c.server.Shutdown(httpCtx), c.hooks.Shutdown(httpCtx))

		drainCtx, cancelDrain := context.WithTimeout(context.Background(), writerDrainGrace)
		defer cancelDrain()
		// Work a handler detached after its 202 writes too — and runs git — so it
		// ends before the app drains, never after the daemon is gone.
		detachedErr := c.api.ShutdownDetached(drainCtx) //nolint:contextcheck // Run's ctx is already done here; the drain has its own budget (above).
		drainErr := c.app.Shutdown(drainCtx)
		return errors.Join(httpErr, detachedErr, drainErr)
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
	// The rebind workers read the stores, so they stop before anything closes.
	c.api.Close()
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
	_ = c.hooks.Close()
	// Released last: the home is held until every store is closed.
	if err := c.session.Close(); err != nil {
		slog.Warn("internal: release the home", "err", err)
	}
}
