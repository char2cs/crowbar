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

// maxRequestBodyBytes caps any single request body (64 MiB). It sits above the
// 25 MiB file-read cap so legitimate file saves and the 2 MiB icon upload pass,
// while a malicious unbounded upload is cut off long before it can exhaust memory.
const maxRequestBodyBytes = 64 << 20

// maxMultipartMemory bounds how much of a multipart upload gin buffers in memory
// before spilling to a temp file. The only multipart route is the 2 MiB-capped
// repo-icon upload, so 8 MiB is ample headroom without a large per-request buffer.
const maxMultipartMemory = 8 << 20

// Container owns the configured gin engine and the v0 surface. The lazy WS
// resource lifecycles are owned by the app layer and torn down via
// app.Container.Close, not here.
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
	router.MaxMultipartMemory = maxMultipartMemory
	router.Use(
		middleware.Logger(),
		middleware.Timing(),
		middleware.Recovery(),
		middleware.CORS(),
		middleware.BodyLimit(maxRequestBodyBytes),
	)

	v0Container := v0.New(appContainer, engContainer)
	v0Container.Register(router.Group("/v0"))
	mountDebug(router)

	if staticFS != nil {
		RegisterStatic(router, staticFS)
	}

	return &Container{router: router, v0: v0Container}, nil
}

// Handler returns the underlying http.Handler.
func (c *Container) Handler() http.Handler {
	return c.router
}
