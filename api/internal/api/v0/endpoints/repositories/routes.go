package repositories

import (
	"github.com/gin-gonic/gin"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repositories/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts repository routes on rg.
//
// Nested routes (POST/GET under /projects/:id/repositories) and standalone
// routes (GET/DELETE under /repositories/:id) are both registered here.
func Register(
	rg *gin.RouterGroup,
	svc usecases.RepositoryUsecase,
) {
	h := repohandlers.New(svc)
	rg.POST("/projects/:id/repositories", h.Create)
	rg.GET("/projects/:id/repositories", h.List)
	rg.GET("/repositories/:id", h.Get)
	rg.DELETE("/repositories/:id", h.Delete)
}
