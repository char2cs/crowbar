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
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/loopback"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/selfinstall"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container owning the HTTP server, its listener, and the
// adapter layer (closed on shutdown).
//
// It may own a SECOND listener: the auxiliary loopback TCP one. Both serve the
// same *http.Handler, so the route surface is identical, but the TCP server's
// handler is wrapped in loopback.Authenticate — a bearer token is mandatory
// there and absent on the unix socket, whose access control is its 0600 file
// mode. The TCP fields are nil when the feature is off, which is the default.
type Container struct {
	server           *http.Server
	listener         net.Listener
	loopbackServer   *http.Server
	loopbackListener net.Listener
	loopbackCreds    string
	engines          *engine.Container
	adapter          *adapter.Container
	app              *app.Container
	api              *crowbarapi.Container
}

type rootOpts struct {
	homeDir      string
	loopbackAddr string
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

// WithLoopbackTCP additionally serves the API on a loopback TCP listener bound
// at addr ("127.0.0.1:0" for an OS-assigned port), on top of the primary
// listener. An empty addr leaves the feature off, which is the default.
//
// The address must be a literal loopback IP or New fails — see
// transports.NewLoopback. Requests to this listener require the bearer token
// published at $CROWBAR_HOME/state/loopback.json; the primary listener is
// untouched.
func WithLoopbackTCP(
	addr string,
) Option {
	return func(o *rootOpts) {
		o.loopbackAddr = addr
	}
}

// New wires engine → adapter → app → api in order and returns the root container.
//
// The listener is bound FIRST, before any of that wiring, so a second daemon
// racing for the same socket loses cheaply. That ordering used to leak the bound
// listener — an fd and, for the unix transport, the socket file itself — whenever
// a later stage failed, which on the unix path then reads as ErrDaemonRunning to
// the NEXT launch: a dead daemon that still owns the address. The named error
// return and its deferred close are what close that hole; every error path below
// releases the listener on its way out.
func New(
	ctx context.Context,
	host string,
	staticFS fs.FS,
	options ...Option,
) (_ *Container, err error) {
	cfg := rootOpts{}
	for _, o := range options {
		o(&cfg)
	}

	listener, err := gateway.New(host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}
	defer closeOnFailure(listener, &err)

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
	if p, installErr := selfinstall.Install(installHome); installErr == nil {
		_ = p // path is re-derived by the agent engine when rendering descriptors
	}

	container := &Container{
		server:   &http.Server{Handler: apiContainer.Handler(), ReadHeaderTimeout: 30 * time.Second},
		listener: listener,
		engines:  engines,
		adapter:  adapters,
		app:      appContainer,
		api:      apiContainer,
	}
	if err = container.enableLoopback(cfg.loopbackAddr, installHome, apiContainer.Handler()); err != nil {
		// The engine, adapter and app layers are already built by this point — live
		// PTYs, file watchers, open sqlite handles. A refused loopback bind must not
		// strand them: Close tears them down in the same dependency order a normal
		// shutdown does, and tolerates never having Run.
		container.Close()
		return nil, err
	}
	return container, nil
}

// enableLoopback binds the auxiliary loopback TCP listener, mints and publishes
// its bearer credential, and builds the second http.Server that serves the SAME
// handler behind loopback.Authenticate. An empty addr is the off switch and the
// default: nothing is bound, no credential is written, and the daemon behaves
// exactly as it did before this listener existed.
//
// The credential is published only AFTER the bind succeeds, using the listener's
// real address, so the file never advertises a port nothing is listening on. A
// failure anywhere here closes what it already opened and propagates, rather than
// leaving the daemon half-listening.
func (c *Container) enableLoopback(
	addr string,
	homeDir string,
	handler http.Handler,
) error {
	if addr == "" {
		return nil
	}
	listener, err := transports.NewLoopback(addr)
	if err != nil {
		return fmt.Errorf("internal: loopback: %w", err)
	}
	credentials, err := loopback.Issue(listener.Addr())
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("internal: loopback: %w", err)
	}
	path, err := credentials.Publish(metadata.GetStateDirPathAt(homeDir))
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("internal: loopback: %w", err)
	}
	c.loopbackListener = listener
	c.loopbackCreds = path
	c.loopbackServer = &http.Server{
		Handler:           loopback.Authenticate(credentials.Token, handler),
		ReadHeaderTimeout: 30 * time.Second,
	}
	return nil
}

// LoopbackAddress returns the address the auxiliary loopback TCP listener is
// bound to ("127.0.0.1:54321"), or "" when the listener is disabled. It reports
// the OS-ASSIGNED port, so a caller that asked for the ephemeral default learns
// the real one without reading the published credentials file.
func (c *Container) LoopbackAddress() string {
	if c.loopbackListener == nil {
		return ""
	}
	return c.loopbackListener.Addr().String()
}

// LoopbackCredentialsPath returns the path of the published credentials file, or
// "" when the loopback listener is disabled.
func (c *Container) LoopbackCredentialsPath() string {
	return c.loopbackCreds
}

func closeOnFailure(
	listener net.Listener,
	err *error,
) {
	if *err != nil {
		_ = listener.Close()
	}
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
//
// When the auxiliary loopback TCP listener is enabled it is served by a SECOND
// goroutine here, and both are shut down together on the way out. serveErr is
// buffered to hold one result PER SERVER, which is what keeps that second
// goroutine from leaking: whichever branch of the select wins, every Serve that
// returns afterwards can deposit its result and exit instead of parking forever
// on a send nobody will ever receive. This daemon has already had one production
// incident from an fd leaked per connection close, so an unjoined goroutine
// holding a server's worth of state is not a theoretical concern here.
func (c *Container) Run(
	ctx context.Context,
) error {
	serveErr := make(chan error, 2)
	go func() { serveErr <- c.server.Serve(c.listener) }()
	if c.loopbackServer != nil {
		go func() { serveErr <- c.loopbackServer.Serve(c.loopbackListener) }()
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
		httpCtx, cancelHTTP := context.WithTimeout(context.Background(), httpDrainGrace)
		defer cancelHTTP()
		// Both servers share the ONE http drain budget rather than getting a budget
		// each: they are the same handler behind two doors, and the point of the small
		// cap is to leave the writer quiesce the majority of the supervisor's 3s window.
		// Shutdown closes each server's own listener, so this is also what stops the
		// loopback port accepting.
		httpErr := errors.Join(c.server.Shutdown(httpCtx), c.shutdownLoopback(httpCtx))

		drainCtx, cancelDrain := context.WithTimeout(context.Background(), writerDrainGrace)
		defer cancelDrain()
		drainErr := c.app.Shutdown(drainCtx)
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
	c.closeLoopback()
}

func (c *Container) shutdownLoopback(
	ctx context.Context,
) error {
	if c.loopbackServer == nil {
		return nil
	}
	return c.loopbackServer.Shutdown(ctx)
}

// closeLoopback releases the auxiliary listener's fd and unpublishes its
// credentials. Revoking matters as much as closing: a token and a port left on
// disk by a daemon that is gone is a live-looking pointer at an address some
// other process may later bind, so the file goes away with the listener.
//
// Closing an already-Shutdown listener is a no-op error we ignore, which is why
// this runs unconditionally on the Close path — the graceful route reaches it
// having already shut the server down, the Serve-failure and never-ran routes
// have not.
func (c *Container) closeLoopback() {
	if c.loopbackListener != nil {
		_ = c.loopbackListener.Close()
	}
	_ = loopback.Revoke(c.loopbackCreds)
}
