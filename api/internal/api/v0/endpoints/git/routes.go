// Package git mounts the v0 git read routes for a workspace: the working-tree
// status (dual-served with the live git WebSocket stream), the paginated log,
// the working-tree or commit diff, the branch list, and the stash list
// (02 §2.6).
package git

import (
	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

// Register mounts the git read routes on the supplied router group. gitSvc backs
// every read. The status route is dual-served: a plain GET returns the REST
// status while a WebSocket upgrade is routed to gitWS for the live stream;
// dispatch wraps the route so the upgrade is honoured without a second path.
func Register(
	rg *gin.RouterGroup,
	gitSvc githandlers.Git,
	gitWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := githandlers.New(gitSvc)
	rg.GET("/workspaces/:wsId/git/status", dispatch(h.Status, gitWS))
	rg.GET("/workspaces/:wsId/git/log", h.Log)
	rg.GET("/workspaces/:wsId/git/diff", h.Diff)
	rg.GET("/workspaces/:wsId/git/branches", h.Branches)
	rg.GET("/workspaces/:wsId/git/stashes", h.Stashes)
}
