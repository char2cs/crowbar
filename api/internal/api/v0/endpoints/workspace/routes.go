// Package workspace mounts the workspace-addressed placement route: a locked branch's own sidebar
// position, moved by workspace id because it is a fact about the workspace, not any chat in it.
package workspace

import (
	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspace/handlers"
)

// Register mounts PATCH .../workspaces/:wsId/placement on the repo-scoped group, whose
// scopeWorkspaceToPath middleware already guards that :wsId belongs to :projectId/:repoId.
func Register(
	repoScoped *gin.RouterGroup,
	placer workspacehandlers.Placer,
) {
	h := workspacehandlers.New(placer)
	rg := repoScoped.Group("/workspaces/:wsId")
	rg.PATCH("/placement", h.PlaceWorkspace)
}
