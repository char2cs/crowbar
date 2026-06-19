// Package system mounts system-level informational routes.
package system

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
)

// Register mounts GET /system/prerequisites. The handler resolves the user's
// login-shell PATH once at startup (5 s timeout) so it is never resolved
// on—or cancelled by—a request context.
func Register(rg *gin.RouterGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h := systemhandlers.NewHandler(ctx)
	rg.GET("/system/prerequisites", h.Prerequisites)
}
