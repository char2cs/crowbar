// Package fixture is the router entry point protogen's tests walk. It nests
// groups the way the daemon's v0 router does, so the walker has to accumulate
// prefixes across a Group chain and across a cross-package Register call.
package fixture

import (
	"github.com/gin-gonic/gin"

	"example.com/fixture/handlers"
	"example.com/fixture/items"
)

// Register mounts every fixture route under rg.
func Register(
	rg *gin.RouterGroup,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := handlers.New()
	rg.GET("/health", h.Untyped)

	projects := rg.Group("/projects")
	scoped := projects.Group("/:projectId")
	items.Register(scoped, wsHandle, dispatch)
}
