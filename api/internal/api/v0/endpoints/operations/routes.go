// Package operations mounts the operations stream and the dismiss verb.
package operations

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/operations/handlers"
)

// Register mounts GET /operations (the WebSocket stream, served by stream) and
// DELETE /operations/:id.
func Register(
	rg *gin.RouterGroup,
	ops handlers.Dismisser,
	stream gin.HandlerFunc,
) {
	h := handlers.New(ops)
	rg.GET("/operations", stream)
	rg.DELETE("/operations/:id", h.Dismiss)
}
