// Package workspaces mounts the v0 workspace REST routes, including the
// worktree hierarchy operations and the dual-served list that upgrades to the
// live workspaces WebSocket stream on demand (02 §2.2).
package workspaces

import (
	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
)

// Register mounts the workspace list, detail, create, delete, and hierarchy
// routes on the supplied router group. The list route is dual-served: a plain
// GET returns the flat REST list while a WebSocket upgrade is routed to wsHandle
// for the live stream. reader and hierarchy back the reads and worktree
// operations; repos resolves a repository for worktree-backed create; dispatch
// wraps the list route so the upgrade is honoured without a second path.
func Register(
	rg *gin.RouterGroup,
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := workspacehandlers.New(reader, hierarchy, repos)
	rg.GET("/workspaces", dispatch(h.List, wsHandle))
	rg.GET("/workspaces/:wsId", h.Detail)
	rg.POST("/workspaces", h.Create)
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rg.POST("/workspaces/:wsId/sync", h.Sync)
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
}
