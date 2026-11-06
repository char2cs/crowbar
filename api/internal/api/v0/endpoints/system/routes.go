// Package system mounts system-level informational routes.
package system

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
)

// Register mounts GET /system/prerequisites/:tool, GET /system/browse (directory listings for the
// client's remote file picker) and the /system/perf arm/read pair. Perf sits on the top-level
// group because the timing ring is process-wide.
func Register(rg *gin.RouterGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h := systemhandlers.NewHandler(ctx)
	rg.GET("/system/prerequisites/:tool", h.Prerequisite)

	rg.GET("/system/browse", systemhandlers.Browse)

	p := systemhandlers.NewPerfHandler()
	rg.GET("/system/perf", p.Get)
	rg.POST("/system/perf", p.Set)
}
