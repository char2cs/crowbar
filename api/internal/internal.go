package internal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/api/middleware"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/bootgate"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/core/selfinstall"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container: the socket, the gate answering on it from
// the moment it binds, and the session holding the home. A migrated home adds
// the stack over its stores; a halted one has none, and its gate says why.
type Container struct {
	home     home.Home
	listener net.Listener
	server   *http.Server
	gate     *bootgate.Gate
	// serveErr carries each Serve's result: the gate's, and the hook server's
	// once Run starts it.
	serveErr chan error
	// serving joins the goroutines running Serve.
	serving sync.WaitGroup
	// session holds the home from before the stores open until after they close.
	session *migrate.Session
	stack   *stack
}

// stack is what a migrated home serves: the engines, the stores and every
// layer over them, and the hook server.
type stack struct {
	engines *engine.Container
	adapter *adapter.Container
	app     *app.Container
	api     *crowbarapi.Container
	// hooks is the loopback listener for vendor CLIs' own HTTP hooks. It serves
	// the hook route of the same router and nothing else of it.
	hooks *hookserver.Server
}

type rootOpts struct {
	homeDir    string
	logs       logring.Ring
	migrateLog *slog.Logger
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

// New serves the boot gate on the socket before it migrates the home, so health
// answers throughout. A halt still yields a container: one that serves only the
// gate and holds the home until it is closed.
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
	h, err := home.ResolveDir(cfg.homeDir)
	if err != nil {
		return nil, fmt.Errorf("internal: home: %w", err)
	}
	listener, err := gateway.New(host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}
	c := &Container{home: h, listener: listener, serveErr: make(chan error, 2)}
	report := &bootReport{}
	c.gate = bootgate.New(metadata.GetVersion(), report.report, middleware.CORSHandler)
	c.server = &http.Server{Handler: c.gate, ReadHeaderTimeout: 30 * time.Second}
	c.serve(func() error { return c.server.Serve(listener) })

	c.session, err = migrations.Begin(ctx, h, migrate.Options{Version: metadata.GetVersion(), Log: cfg.migrateLog})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("internal: migrate: %w", err), c.release())
	}
	// Published before any step runs, so the health route names each one.
	report.session.Store(c.session)
	if err := c.boot(ctx, cfg, staticFS); err != nil {
		if c.session.Halt() == nil {
			return nil, errors.Join(err, c.release())
		}
		warnHaltedBoot(err)
	}
	return c, nil
}

// warnHaltedBoot logs a halted boot's error when it carries more than the halt,
// such as a close failure: serve announces the halt alone.
func warnHaltedBoot(
	err error,
) {
	if besidesHalt(err) {
		slog.Warn("internal: boot halted, and more failed", "err", err)
	}
}

// besidesHalt reports whether err's tree has a leaf that is not a Halt.
func besidesHalt(
	err error,
) bool {
	if _, ok := err.(*migrate.Halt); ok { //nolint:errorlint // the tree is walked node by node: a wrapped Halt is reached below
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return slices.ContainsFunc(joined.Unwrap(), besidesHalt)
	}
	if next := errors.Unwrap(err); next != nil {
		return besidesHalt(next)
	}
	return true
}

// boot stops at a home Begin refused. Otherwise it runs the pre-open migrations,
// opens the stack over the stores (app.New runs the post-open ones) and opens
// the gate to its API.
func (c *Container) boot(
	ctx context.Context,
	cfg rootOpts,
	staticFS fs.FS,
) error {
	if halt := c.session.Halt(); halt != nil {
		return halt
	}
	if err := c.session.RunPreOpen(ctx, migrations.PreOpenImpls()); err != nil {
		return fmt.Errorf("internal: migrate: %w", err)
	}
	s, err := openStack(ctx, c.home, cfg, socketOf(c.listener), c.session, staticFS)
	if err != nil {
		return err
	}
	c.stack = s
	c.gate.Open(s.api.Handler())
	return nil
}

// openStack builds the stack in order. On failure it closes what it had opened
// and returns any close failure with the error, so the stores are closed before
// the caller lets go of the home.
func openStack(
	ctx context.Context,
	h home.Home,
	cfg rootOpts,
	socket string,
	post migrate.PostOpenRunner,
	staticFS fs.FS,
) (*stack, error) {
	s := &stack{}
	if err := s.open(ctx, h, cfg, socket, post, staticFS); err != nil {
		//nolint:contextcheck // the Close methods take no ctx: a failed boot's teardown runs even after ctx is done.
		return nil, errors.Join(err, s.close())
	}
	return s, nil
}

func (s *stack) open(
	ctx context.Context,
	h home.Home,
	cfg rootOpts,
	socket string,
	post migrate.PostOpenRunner,
	staticFS fs.FS,
) error {
	var err error
	s.engines, err = engine.New(ctx, engine.WithHomeDir(cfg.homeDir), engine.WithDaemonSocket(socket))
	if err != nil {
		return fmt.Errorf("internal: engine: %w", err)
	}
	s.adapter, err = adapter.New(adapter.WithHomeDir(h.Root()))
	if err != nil {
		return fmt.Errorf("internal: adapter: %w", err)
	}
	s.app, err = app.New(ctx, s.engines, s.adapter, post)
	if err != nil {
		return fmt.Errorf("internal: app: %w", err)
	}
	//nolint:contextcheck // snapshot providers are context-free by design; this call only threads the log ring option.
	s.api, err = crowbarapi.New(s.app, s.engines, staticFS, v0.WithLogs(cfg.logs))
	if err != nil {
		return fmt.Errorf("internal: api: %w", err)
	}

	// Install the crowbar binary into <home>/bin so vendor CLI hooks can invoke
	// `crowbar hook` by absolute path, into the home the adapter opened so tests
	// and dev instances stay isolated. Best-effort: never block startup.
	installHome := s.adapter.CrowbarHome()
	if _, err := selfinstall.Install(installHome); err != nil {
		slog.Warn("internal: install the crowbar binary for hooks", "home", installHome, "err", err)
	}

	// Published before anything can spawn a CLI: the spawner reads it to point the
	// CLI's hooks here.
	s.hooks, err = hookserver.Listen(ctx, s.api.Handler())
	if err != nil {
		return fmt.Errorf("internal: hooks: %w", err)
	}
	if err := hookserver.Publish(installHome, s.hooks.URL()); err != nil {
		return fmt.Errorf("internal: hooks: %w", err)
	}
	return nil
}

// close tears the stack down in dependency order, skipping the layers a failed
// boot never built. The app layer's watchers and LSP hosts live on hijacked
// connections, which http.Server.Shutdown leaves running.
func (s *stack) close() error {
	var errs []error
	if s.hooks != nil {
		errs = append(errs, s.hooks.Close())
	}
	// The rebind workers read the stores, so they stop before anything closes.
	if s.api != nil {
		s.api.Close()
	}
	if s.app != nil {
		s.app.Close()
	}
	// Kills live PTY children so no terminal's shell outlives the daemon. The graceful
	// path quiesced them already (app.Shutdown); this covers a Serve failure or a
	// Close without Run, and never re-enters that join.
	if s.engines != nil {
		s.engines.Close()
	}
	// The DBs go LAST, once every writer above is quiesced: the exit callbacks fired
	// by the PTY teardown write runner Exits and turn closes, and closing the store
	// out from under them is what used to leave a chat spinning forever.
	if s.adapter != nil {
		errs = append(errs, s.adapter.Close())
	}
	return errors.Join(errs...)
}

// serve runs a server's Serve, reporting its result on serveErr.
func (c *Container) serve(
	run func() error,
) {
	c.serving.Go(func() { c.serveErr <- run() })
}

// release undoes a New that fails: the gate stops serving, then the home is
// let go. The stores, if any were opened, are closed by then.
func (c *Container) release() error {
	err := c.server.Close()
	c.serving.Wait()
	if c.session != nil {
		err = errors.Join(err, c.session.Close())
	}
	return err
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
	return socketOf(c.listener)
}

// Home is the Crowbar home the daemon holds.
func (c *Container) Home() string {
	return c.home.Root()
}

// Halt is why the daemon serves only its gate, or nil when it serves the API.
func (c *Container) Halt() *migrate.Halt {
	return c.session.Halt()
}

// Run serves until ctx is cancelled, then gracefully shuts down. It also watches
// for an early Serve failure: if Serve returns before shutdown is requested
// (listener invalidated, accept failing permanently under FD pressure), the
// error is surfaced so the process exits non-zero and its supervisor restarts
// the daemon — rather than silently hanging "up" while accepting no connections.
func (c *Container) Run(
	ctx context.Context,
) error {
	if c.stack != nil {
		c.serve(c.stack.hooks.Serve)
	}
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
		httpErr := c.server.Shutdown(httpCtx)
		if c.stack == nil {
			// A halted daemon opened no store, so it has no writer to drain.
			return httpErr
		}
		httpErr = errors.Join(httpErr, c.stack.hooks.Shutdown(httpCtx))

		drainCtx, cancelDrain := context.WithTimeout(context.WithoutCancel(ctx), writerDrainGrace)
		defer cancelDrain()
		// Work a handler detached after its 202 writes too — and runs git — so it
		// ends before the app drains, never after the daemon is gone.
		detachedErr := c.stack.api.ShutdownDetached(drainCtx)
		drainErr := c.stack.app.Shutdown(drainCtx)
		return errors.Join(httpErr, detachedErr, drainErr)
	case err := <-c.serveErr:
		// ErrServerClosed only occurs after Shutdown (handled above), so any error
		// arriving here is a genuine listen/accept failure.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("internal: serve: %w", err)
		}
		return nil
	}
}

// Close stops serving and tears the backend down in dependency order (Run has
// usually shut the servers down already), then lets go of the home.
func (c *Container) Close() {
	errs := []error{c.server.Close()}
	if c.stack != nil {
		errs = append(errs, c.stack.close())
	}
	c.serving.Wait()
	// Released last: the home is held until every store is closed.
	errs = append(errs, c.session.Close())
	if err := errors.Join(errs...); err != nil {
		slog.Warn("internal: close", "err", err)
	}
}
