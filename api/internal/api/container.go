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

const maxRequestBodyBytes = 64 << 20

const maxMultipartMemory = 8 << 20

type Container struct {
	router *gin.Engine
}

func New(
	appContainer *app.Container,
	engContainer *engine.Container,
	staticFS fs.FS,
	access v0.Access,
	preAuthReadTimeout time.Duration,
	v0Options ...v0.Option,
) (*Container, error) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.UseRawPath = true
	router.UnescapePathValues = true
	router.MaxMultipartMemory = maxMultipartMemory
	router.Use(
		middleware.PreAuthReadDeadline(preAuthReadTimeout),
		middleware.Logger(),
		middleware.Timing(),
		middleware.Recovery(),
		middleware.CORS(),
		middleware.BodyLimit(maxRequestBodyBytes),
	)

	v0Container := v0.New(appContainer, engContainer, access, v0Options...)
	v0Container.Register(router.Group("/v0"))
	mountDebug(router)

	if staticFS != nil {
		RegisterStatic(router, staticFS)
	}

	return &Container{router: router}, nil
}

func (c *Container) Handler() http.Handler {
	return c.router
}
