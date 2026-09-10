// Package workspace mounts the workspace-addressed placement route: a LOCKED
// branch's own sidebar position, moved by the workspace's own id (2026-09-09
// sidebar-placement-unification, workspace-placement fix). See
// handlers.Handlers' own doc for why this is the SECOND genuinely
// workspace-native route in the daemon, not the first — threads
// (endpoints/threads) already reuses the identical repoScoped
// "/workspaces/:wsId" mount for the same reason: a fact about the workspace
// itself, not about any conversation inside it.
package workspace

import (
	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspace/handlers"
)

// Register mounts PATCH .../workspaces/:wsId/placement on the repo-scoped
// group. scopeWorkspaceToPath (router.go) already guards :wsId ⊂
// :projectId/:repoId for every route mounted here, installed on repoScoped
// itself before this group is derived — the same guard threads.Register's
// identical mount relies on.
func Register(
	repoScoped *gin.RouterGroup,
	placer workspacehandlers.Placer,
	broadcastFolder func(folderID, workspaceID, kind string),
) {
	h := workspacehandlers.New(placer, broadcastFolder)
	rg := repoScoped.Group("/workspaces/:wsId")
	rg.PATCH("/placement", h.PlaceWorkspace)
}
