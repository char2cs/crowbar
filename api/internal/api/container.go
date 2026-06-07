package api

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/middleware"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container owns the configured gin engine and the v0 surface whose lazy WS
// resource lifecycles must be torn down on shutdown.
type Container struct {
	router *gin.Engine
	v0     *v0.Container
}

// New builds the HTTP layer: middleware, the v0 surface (mounted at /v0 and
// registered as a hub subscriber), and optional embedded static assets.
func New(
	appContainer *app.Container,
	engContainer *engine.Container,
	staticFS fs.FS,
) (*Container, error) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.UseRawPath = true
	router.UnescapePathValues = true
	router.Use(middleware.Logger(), middleware.Recovery())

	v0Container := v0.New(appContainer, engContainer)
	v0Container.Register(router.Group("/v0"))

	if staticFS != nil {
		RegisterStatic(router, staticFS)
	}

	return &Container{router: router, v0: v0Container}, nil
}

// Handler returns the underlying http.Handler.
func (c *Container) Handler() http.Handler {
	return c.router
}

// Close tears down the v0 surface's lazy WS resource lifecycles (file watchers
// and LSP hosts). It is called after the HTTP server has shut down, since
// http.Server.Shutdown does not force-close hijacked WebSocket connections and
// so never drives the per-workspace OnUnsubscribe teardown. It is idempotent.
func (c *Container) Close() {
	c.v0.Close()
}
