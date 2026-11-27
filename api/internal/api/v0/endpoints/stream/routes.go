package stream

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/stream/handlers"
)

func Register(rg *gin.RouterGroup, hub handlers.Server) {
	rg.GET("/stream", handlers.New(hub, handlers.DefaultKeepAlive).Stream)
}
