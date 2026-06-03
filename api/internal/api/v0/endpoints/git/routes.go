package git

import (
	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all git routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.GitUsecase,
) {
	h := githandlers.New(svc)
	rg.GET("/tasks/:id/git/log", h.Log)
	rg.GET("/tasks/:id/git/diff", h.Diff)
	rg.GET("/tasks/:id/files", h.ListFiles)
}
