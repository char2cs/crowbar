package projects

import (
	"github.com/gin-gonic/gin"

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all project routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.ProjectUsecase,
) {
	h := projecthandlers.New(svc)
	rg.POST("", h.Create)
	rg.GET("", h.List)
	rg.GET("/:id", h.Get)
	rg.DELETE("/:id", h.Delete)
}
