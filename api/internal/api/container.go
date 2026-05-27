package api

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/api/middleware"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
)

type Container struct {
	router *gin.Engine
}

func New(appContainer *app.Container, staticFS fs.FS) (*Container, error) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.Logger(), middleware.Recovery())

	apiV0 := router.Group("/api/v0")
	v0.Register(apiV0, appContainer)

	if staticFS != nil {
		RegisterStatic(router, staticFS)
	}

	return &Container{router: router}, nil
}

func (c *Container) Handler() http.Handler { return c.router }
