// Package provider mounts the v0 provider REST routes: workspace provider state
// poll and protected-branch lookup.
package provider

import (
	"github.com/gin-gonic/gin"

	provhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
)

// Register mounts the provider routes on the supplied router group.
func Register(
	rg *gin.RouterGroup,
	provEng provhandlers.ProviderEngine,
	wsReader provhandlers.WorkspaceReader,
) {
	h := provhandlers.New(provEng, wsReader)
	rg.GET("/workspaces/:wsId/provider", h.State)
	rg.GET("/repos/:id/protected-branches", h.ProtectedBranches)
}
