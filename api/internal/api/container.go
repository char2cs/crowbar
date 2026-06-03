package api

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/api/middleware"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
)

// Container holds the HTTP server and the Gin engine.
type Container struct {
	server *http.Server
}

// New constructs the API container: creates the Gin engine, registers all
// v0 routes, wires the WebSocket hub subscriber, and mounts the MCP server.
// staticFS is the embedded web UI filesystem; if nil, static serving is skipped.
func New(
	appContainer *app.Container,
	staticFS fs.FS,
) (*Container, error) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Logger(), middleware.Recovery())

	apiV0 := router.Group("/api/v0")
	apiV0.Use(middleware.Chaos())
	v0.Register(apiV0, appContainer)

	// Register the WS handler as a hub subscriber so it receives domain events.
	appContainer.Hub.Register(wsHandler)

	// Register all v0 REST + WS routes.
	v0Router := v0.NewRouter(appContainer, wsHandler)
	v0Router.Register(r.Group("/api/v0"))

	// Mount the MCP server at /mcp (root-level, not under /api/v0).
	appContainer.Repos.MCP.Mount(r.Group("/mcp"))

	// Mount static web assets when the embedded FS is provided.
	if staticFS != nil {
		RegisterStatic(r, staticFS)
	}

	return &Container{
		server: &http.Server{Handler: r},
	}, nil
}

// Run starts serving on the provided listener. It blocks until the server
// is closed. The caller should check for http.ErrServerClosed.
func (c *Container) Run(l net.Listener) error {
	return c.server.Serve(l)
}

// ServeOn starts serving on an additional listener in a background goroutine
// using the same http.Server (so Shutdown closes it too). Returns a channel
// that receives the error when the listener closes.
func (c *Container) ServeOn(l net.Listener) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- c.server.Serve(l) }()
	return ch
}

// Shutdown gracefully drains in-flight requests within the given deadline.
func (c *Container) Shutdown(ctx context.Context) error {
	return c.server.Shutdown(ctx)
}

// ShutdownDefault is a convenience wrapper with a 5-second timeout.
func (c *Container) ShutdownDefault() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Shutdown(ctx)
}
