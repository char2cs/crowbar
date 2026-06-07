package internal

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container owning the HTTP server, its listener, and the
// adapter layer (closed on shutdown).
type Container struct {
	server   *http.Server
	listener net.Listener
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

	return &Container{
		server:   &http.Server{Handler: apiContainer.Handler(), ReadHeaderTimeout: 30 * time.Second},
		listener: listener,
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

// Run serves until ctx is cancelled, then gracefully shuts down.
func (c *Container) Run(
	ctx context.Context,
) error {
	go c.server.Serve(c.listener) //nolint:errcheck
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.server.Shutdown(shutdownCtx)
}

// Close tears the backend down in dependency order: the HTTP server has already
// stopped accepting connections (Run shuts it down before Close runs), so it
// first closes the app layer's lazy realtime resource lifecycles (file watchers
// and LSP hosts, which http.Server.Shutdown leaves running on hijacked WebSocket
// connections), then releases the adapter layer and the listener.
func (c *Container) Close() {
	c.app.Close()
	_ = c.adapter.Close()
	_ = c.listener.Close()
}
