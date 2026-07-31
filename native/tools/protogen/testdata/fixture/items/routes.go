// Package items mounts the fixture's item routes on a caller-supplied group,
// mirroring the daemon's endpoints/<name>/routes.go shape.
package items

import (
	"github.com/gin-gonic/gin"

	"example.com/fixture/handlers"
)

// Register mounts the item routes. dispatch is the dual-serve wrapper: a plain
// GET answers REST while a WebSocket upgrade is routed to ws, so the REST DTO
// lives in its FIRST argument.
func Register(
	rg *gin.RouterGroup,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := handlers.New()
	g := rg.Group("/items")

	g.GET("", dispatch(h.ListItems, wsHandle))
	g.GET("/:id", h.GetItem)
	g.POST("", h.CreateItem)
	g.PATCH("/:id", h.RenameItem)
	g.DELETE("/:id", h.DeleteItem)
	g.GET("/ws", wsHandle)

	rg.GET("/tree", h.Tree)
	rg.GET("/patch", h.Patch)
	rg.GET("/untyped", h.Untyped)
	rg.GET("/lossy", h.Lossy)
	rg.POST("/stage", h.Stage)
}
