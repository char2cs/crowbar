package api

import (
	"io/fs"
	"net/http"
	"time"

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

// Container owns the configured gin engine with the v0 surface mounted.
type Container struct {
	router *gin.Engine
}

// New builds the HTTP layer: middleware, the v0 surface at /v0, and optional static
// assets. serverFingerprint (the TLS identity) feeds the pairing routes; preAuthReadTimeout
// bounds a TLS request's reads until its bearer is verified.
func New(
	appContainer *app.Container,
	engContainer *engine.Container,
	staticFS fs.FS,
	serverFingerprint string,
	preAuthReadTimeout time.Duration,
	v0Options ...v0.Option,
) (*Container, error) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.UseRawPath = true
	router.UnescapePathValues = true
	router.MaxMultipartMemory = maxMultipartMemory
	router.Use(
		// First, so every route an anonymous TLS peer can reach is bounded:
		// pairing, pprof, static, a CORS refusal, and the drain after a 401.
		middleware.PreAuthReadDeadline(preAuthReadTimeout),
		middleware.Logger(),
		middleware.Timing(),
		middleware.Recovery(),
		middleware.CORS(),
		middleware.BodyLimit(maxRequestBodyBytes),
	)

	v0Container := v0.New(appContainer, engContainer, serverFingerprint, v0Options...)
	v0Container.Register(router.Group("/v0"))
	mountDebug(router)

	if staticFS != nil {
		RegisterStatic(router, staticFS)
	}

	return &Container{router: router}, nil
}

// Handler returns the underlying http.Handler.
func (c *Container) Handler() http.Handler {
	return c.router
}
