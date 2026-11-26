// Package access mounts the route that opens and closes device connections.
package access

import (
	"github.com/gin-gonic/gin"

	accesshandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/access/handlers"
)

// Register mounts GET and PUT /access, guarded like every route past the bearer middleware.
func Register(
	guarded *gin.RouterGroup,
	h *accesshandlers.Handlers,
) {
	guarded.GET("/access", h.Get)
	guarded.PUT("/access", h.Put)
}
