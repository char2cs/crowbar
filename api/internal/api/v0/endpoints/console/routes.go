package console

import (
	"github.com/gin-gonic/gin"

	consolehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/console/handlers"
	"github.com/char2cs/crowbar/api/internal/core/logring"
)

func Register(
	rg *gin.RouterGroup,
	logs logring.Ring,
) {
	rg.GET("/console/logs", consolehandlers.New(logs).Logs)
}
