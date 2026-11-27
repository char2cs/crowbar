package access

import (
	"github.com/gin-gonic/gin"

	accesshandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/access/handlers"
)

func Register(
	guarded *gin.RouterGroup,
	h *accesshandlers.Handlers,
) {
	guarded.GET("/access", h.Get)
	guarded.PUT("/access", h.Put)
}
