package system

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
)

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
