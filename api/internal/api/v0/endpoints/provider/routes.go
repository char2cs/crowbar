package provider

import (
	"github.com/gin-gonic/gin"

	provhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
)

func Register(
	repoScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	provEng provhandlers.ProviderEngine,
	wsReader provhandlers.WorkspaceReader,
) {
	h := provhandlers.New(provEng, wsReader)
	chatScoped.GET("/provider", h.State)
	repoScoped.GET("/protected-branches", h.ProtectedBranches)
}
