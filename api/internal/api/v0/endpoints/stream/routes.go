// Package stream mounts the one daemon-to-client stream.
package stream

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/stream/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// Register mounts GET /stream.
func Register(rg *gin.RouterGroup, hub handlers.Server) {
	rg.GET("/stream", handlers.New(hub, ws.DefaultKeepAlive).Stream)
}
