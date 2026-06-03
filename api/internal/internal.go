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

// Container holds all internal layer dependencies.
type Container struct {
	Engines  *engine.Container
	Adapters *adapter.Container
	App      *app.Container
	API      *crowbarapi.Container

	runErr chan error
}

// New wires all internal layers in order: engine → adapter → app → api.
// host is the listen address (e.g. "unix:///tmp/crowbar.sock" or "tcp://0.0.0.0:3737").
// staticFS is the embedded web UI filesystem; pass nil to skip static serving.
func New(
	ctx context.Context,
	host string,
	staticFS fs.FS,
) (*Container, error) {
	chatHub := hub.NewChatHub()

	// Bind a random TCP port so agent subprocesses can reach /mcp regardless
	// of whether the main server is a Unix socket or TCP.
	mcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("internal: mcp listener: %w", err)
	}
	mcpURL := "http://" + mcpListener.Addr().String() + "/mcp"

	engines, err := engine.New(ctx, chatHub, engine.WithMCPURL(mcpURL))
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New()
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	appContainer, err := app.New(ctx, engines, adapters, app.WithChatHub(chatHub))
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	apiContainer, err := crowbarapi.New(appContainer, staticFS)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	listener, err := gateway.New(host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}

	apiContainer.ServeOn(mcpListener)

	runErr := make(chan error, 1)
	go func() { runErr <- apiContainer.Run(listener) }()

	return &Container{
		Engines:  engines,
		Adapters: adapters,
		App:      appContainer,
		API:      apiContainer,
		runErr:   runErr,
	}, nil
}

// Run blocks until ctx is cancelled, then shuts down gracefully.
func (c *Container) Run(
	ctx context.Context,
) error {
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.API.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("internal: shutdown: %w", err)
	}

	if err := <-c.runErr; err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("internal: api: %w", err)
	}

	return nil
}

// Close releases all adapter resources (SQLite file handles, WAL checkpoints).
func (c *Container) Close() error {
	if err := c.Adapters.Close(); err != nil {
		return fmt.Errorf("internal: adapters close: %w", err)
	}
	return nil
}
