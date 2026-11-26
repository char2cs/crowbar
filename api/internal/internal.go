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
	"github.com/char2cs/crowbar/api/internal/core/exposure"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container: the socket, the gate answering on it from
// the moment it binds, and the session holding the home. A migrated home adds
// the stack over its stores; a halted one has none, and its gate says why.
type Container struct {
	home home.Home
	// listeners are the local binds of the host; the first is the one callbacks dial.
	listeners []net.Listener
	// exposure is the TLS listener other devices reach the daemon on, opened by
	// the host flag or the owner's saved choice and switched while running.
	exposure *exposure.Exposure
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
	// An exposed host is the device listener's to open; the daemon's own binds stay local.
	bootExposed := ""
	if gateway.Exposed(host) {
		bootExposed, host = host, "unix://"
	}
	listeners, err := gateway.New(host, h.Socket())
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}
	c := &Container{home: h, listeners: listeners, serveErr: make(chan error, len(listeners)+1)}
	report := &bootReport{}
	c.gate = bootgate.New(metadata.GetVersion(), report.report, middleware.CORSHandler)
	c.server = newHTTPServer(c.gate, idleConnTimeout)
	for _, ln := range listeners {
		c.serve(func() error { return c.server.Serve(ln) })
	}
	c.exposure = exposure.New(exposure.Config{
		Root:       h.Root(),
		NewServer:  func() *http.Server { return newHTTPServer(c.gate, idleConnTimeout) },
		Listen:     exposure.NetListen,
		Interfaces: net.InterfaceAddrs,
	})
	if err := c.exposure.Start(bootExposed); err != nil {
		return nil, errors.Join(fmt.Errorf("internal: device listener: %w", err), c.release())
	}

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

// idleConnTimeout closes a keep-alive connection left idle between requests,
// so an unauthenticated peer cannot pin a descriptor per connection forever.
const idleConnTimeout = 2 * time.Minute

// preAuthReadTimeout bounds the reads of a TLS request until its bearer is
// verified: the mid-request counterpart of idleConnTimeout.
const preAuthReadTimeout = 10 * time.Second

// newHTTPServer leaves ReadTimeout and WriteTimeout unset: a hijacked WebSocket
// or a long streaming response must outlive any fixed deadline. An anonymous
// request's reads are bounded per request instead (preAuthReadTimeout).
func newHTTPServer(
	handler http.Handler,
	idleTimeout time.Duration,
) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 30 * time.Second, IdleTimeout: idleTimeout}
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
	s, err := openStack(ctx, c.home, cfg, c.Address(), c.exposure, c.session, staticFS)
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
	address string,
	access v0.Access,
	post migrate.PostOpenRunner,
	staticFS fs.FS,
) (*stack, error) {
	s := &stack{}
	if err := s.open(ctx, h, cfg, address, access, post, staticFS); err != nil {
		//nolint:contextcheck // the Close methods take no ctx: a failed boot's teardown runs even after ctx is done.
		return nil, errors.Join(err, s.close())
	}
	return s, nil
}

func (s *stack) open(
	ctx context.Context,
	h home.Home,
	cfg rootOpts,
	address string,
	access v0.Access,
	post migrate.PostOpenRunner,
	staticFS fs.FS,
) error {
	var err error
	s.engines, err = engine.New(ctx, engine.WithHomeDir(cfg.homeDir), engine.WithDaemonAddress(address))
	if err != nil {
		return fmt.Errorf("internal: engine: %w", err)
	}
	s.adapter, err = adapter.New(adapter.WithHomeDir(h.Root()))
	if err != nil {
		return fmt.Errorf("internal: adapter: %w", err)
	}
	s.app, err = app.New(ctx, s.engines, s.adapter, post, v0.EncodeEntity, v0.NewSidebar)
	if err != nil {
		return fmt.Errorf("internal: app: %w", err)
	}
	s.api, err = crowbarapi.New(s.app, s.engines, staticFS, access, preAuthReadTimeout, v0.WithLogs(cfg.logs))
	if err != nil {
		return fmt.Errorf("internal: api: %w", err)
	}

	// Published before anything can spawn a CLI: the spawner reads it to point the
	// CLI's hooks here.
	s.hooks, err = hookserver.Listen(ctx, s.api.Handler())
	if err != nil {
		return fmt.Errorf("internal: hooks: %w", err)
	}
	if err := hookserver.Publish(s.adapter.Home().Root(), s.hooks.URL()); err != nil {
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
	if s.app != nil {
		s.app.Close()
	}
	// Kills live PTY children so no terminal's shell outlives the daemon. The graceful
	// path quiesced them already (app.Shutdown); this covers a Serve failure or a
	// Close without Run, and never re-enters that join.
	if s.engines != nil {
		s.engines.Close()
	}
	// The DBs close last, once every writer is quiesced: the PTY teardown's exit callbacks
	// write runner exits and turn closes, and a closed store would leave a chat spinning.
	if s.adapter != nil {
		errs = append(errs, s.adapter.Close())
	}
	return errors.Join(errs...)
}

// addressOf is the address listener is bound to, as a callback's --host:
// unix://<path>, or tcp://<host:port> with the port the bind chose. Only a
// loopback TCP bind is ever first: an exposed one is the device listener's, not a local bind.
func addressOf(
	listener net.Listener,
) string {
	if addr, ok := listener.Addr().(*net.UnixAddr); ok {
		return "unix://" + addr.Name
	}
	return "tcp://" + listener.Addr().String()
}

// Address is the address callbacks dial: the home's socket whenever it is bound.
func (c *Container) Address() string {
	return addressOf(c.listeners[0])
}

// Home is the Crowbar home the daemon holds.
func (c *Container) Home() string {
	return c.home.Root()
}

// Halt is why the daemon serves only its gate, or nil when it serves the API.
func (c *Container) Halt() *migrate.Halt {
	return c.session.Halt()
}
