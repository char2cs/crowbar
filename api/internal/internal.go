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

type Container struct {
	home      home.Home
	listeners []net.Listener
	exposure  *exposure.Exposure
	server    *http.Server
	gate      *bootgate.Gate
	serveErr  chan error
	serving   sync.WaitGroup
	session   *migrate.Session
	stack     *stack
}

type stack struct {
	engines *engine.Container
	adapter *adapter.Container
	app     *app.Container
	api     *crowbarapi.Container
	hooks   *hookserver.Server
}

type rootOpts struct {
	homeDir    string
	logs       logring.Ring
	migrateLog *slog.Logger
}

type Option func(*rootOpts)

func WithHomeDir(
	dir string,
) Option {
	return func(o *rootOpts) {
		o.homeDir = dir
	}
}

func WithLogRing(
	ring logring.Ring,
) Option {
	return func(o *rootOpts) {
		o.logs = ring
	}
}

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
	report.session.Store(c.session)
	if err := c.boot(ctx, cfg, staticFS); err != nil {
		if c.session.Halt() == nil {
			return nil, errors.Join(err, c.release())
		}
		warnHaltedBoot(err)
	}
	return c, nil
}

const idleConnTimeout = 2 * time.Minute

const preAuthReadTimeout = 10 * time.Second

func newHTTPServer(
	handler http.Handler,
	idleTimeout time.Duration,
) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 30 * time.Second, IdleTimeout: idleTimeout}
}

func warnHaltedBoot(
	err error,
) {
	if besidesHalt(err) {
		slog.Warn("internal: boot halted, and more failed", "err", err)
	}
}

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

	s.hooks, err = hookserver.Listen(ctx, s.api.Handler())
	if err != nil {
		return fmt.Errorf("internal: hooks: %w", err)
	}
	if err := hookserver.Publish(s.adapter.Home().Root(), s.hooks.URL()); err != nil {
		return fmt.Errorf("internal: hooks: %w", err)
	}
	return nil
}

func (s *stack) close() error {
	var errs []error
	if s.hooks != nil {
		errs = append(errs, s.hooks.Close())
	}
	if s.app != nil {
		s.app.Close()
	}
	if s.engines != nil {
		s.engines.Close()
	}
	if s.adapter != nil {
		errs = append(errs, s.adapter.Close())
	}
	return errors.Join(errs...)
}

func addressOf(
	listener net.Listener,
) string {
	if addr, ok := listener.Addr().(*net.UnixAddr); ok {
		return "unix://" + addr.Name
	}
	return "tcp://" + listener.Addr().String()
}

func (c *Container) Address() string {
	return addressOf(c.listeners[0])
}

func (c *Container) Home() string {
	return c.home.Root()
}

func (c *Container) Halt() *migrate.Halt {
	return c.session.Halt()
}
