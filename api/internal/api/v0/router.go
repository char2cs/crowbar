package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/crowbar/api/internal/app"
)

func Register(rg *gin.RouterGroup, appContainer *app.Container) {
	health := NewHealthHandler(appContainer.Health)
	events := NewEventsHandler(appContainer.Hub)

	rg.GET("/health", health.Check)
	rg.GET("/events", events.Stream)
}
