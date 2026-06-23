// Package workspaces mounts the v0 workspace REST routes, including the
// worktree hierarchy operations and the dual-served list that upgrades to the
// live workspaces WebSocket stream on demand (02 §2.2).
package workspaces

import (
	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
)

// Register mounts the workspace list, detail, create, delete, and hierarchy
// routes on the supplied router group. The list AND detail routes are
// dual-served: a plain GET returns REST while a WebSocket upgrade is routed to
// wsHandle for the live stream — the list-scope subscriber receives the repo's
// workspaces, the :wsId-scope subscriber receives exactly that workspace (W7-2).
// reader and hierarchy back the reads and worktree operations; repos resolves a
// repository for worktree-backed create; dispatch wraps the dual-served routes
// so the upgrade is honoured without a second path.
func Register(
	rg *gin.RouterGroup,
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
	lastErrors workspacehandlers.LastErrorSetter,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := workspacehandlers.New(reader, hierarchy, repos, lastErrors)
	rg.GET("/workspaces", dispatch(h.List, wsHandle))
	rg.GET("/workspaces/:wsId", dispatch(h.Detail, wsHandle))
	rg.POST("/workspaces", h.Create)
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rg.POST("/workspaces/:wsId/sync", h.Sync)
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	rg.POST("/workspaces/:wsId/rebase-onto-parent", h.RebaseOntoParent)
}
