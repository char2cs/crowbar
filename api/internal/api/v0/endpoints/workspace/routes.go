package workspace

import (
	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspace/handlers"
)

func Register(
	repoScoped *gin.RouterGroup,
	placer workspacehandlers.Placer,
) {
	h := workspacehandlers.New(placer)
	rg := repoScoped.Group("/workspaces/:wsId")
	rg.PATCH("/placement", h.PlaceWorkspace)
}
