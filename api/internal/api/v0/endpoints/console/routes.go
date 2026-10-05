// Package console mounts the log console route.
package console

import (
	"github.com/gin-gonic/gin"

	consolehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/console/handlers"
	"github.com/char2cs/crowbar/api/internal/core/logring"
)

// Register mounts GET /console/logs, a WebSocket of the daemon's log records.
func Register(
	rg *gin.RouterGroup,
	logs logring.Ring,
) {
	rg.GET("/console/logs", consolehandlers.New(logs).Logs)
}
