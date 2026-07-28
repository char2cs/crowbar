// Package system mounts system-level informational routes.
package system

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
)

// Register mounts GET /system/prerequisites and the /system/perf pair. The
// prerequisites handler resolves the user's login-shell PATH once at startup
// (5 s timeout) so it is never resolved on—or cancelled by—a request context.
//
// /system/perf is the read/arm seam for the daemon's timing ring: a capture run
// POSTs to arm it, drives a scenario, and GETs the samples back. It mounts here
// on the top-level group rather than under the entity hierarchy because the
// ring is process-wide, not scoped to a project, repo or workspace.
func Register(rg *gin.RouterGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h := systemhandlers.NewHandler(ctx)
	rg.GET("/system/prerequisites", h.Prerequisites)

	p := systemhandlers.NewPerfHandler()
	rg.GET("/system/perf", p.Get)
	rg.POST("/system/perf", p.Set)
}
