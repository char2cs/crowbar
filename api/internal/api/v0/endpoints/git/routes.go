// Package git mounts the v0 git routes for a workspace: the read routes (the
// working-tree status dual-served with the live git WebSocket stream, the
// paginated log, the working-tree or commit diff, the branch list, and the
// stash list — 02 §2.6) and the write routes (staging, commit, sync, branch,
// stash, and reset/merge/rebase mutations — 02 §2.7).
//
// Every write route returns the uniform mutation envelope keyed by the affected
// entity — the workspace — via libs.WriteMutationOK: status 200 for in-place
// mutations and 201 only for branch creation, which adds a ref.
package git

import (
	"time"

	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

// Register mounts the git read and write routes on the supplied router group.
// gitSvc backs every route. The status route is dual-served: a plain GET returns
// the REST status while a WebSocket upgrade is routed to gitWS for the live
// stream; dispatch wraps the route so the upgrade is honoured without a second
// path. Write handlers source their resync timestamp from time.Now.
func Register(
	rg *gin.RouterGroup,
	gitSvc githandlers.Git,
	gitWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := githandlers.New(gitSvc, time.Now)
	rg.GET("/workspaces/:wsId/git/status", dispatch(h.Status, gitWS))
	rg.GET("/workspaces/:wsId/git/log", h.Log)
	rg.GET("/workspaces/:wsId/git/diff", h.Diff)
	rg.GET("/workspaces/:wsId/git/branches", h.Branches)
	rg.GET("/workspaces/:wsId/git/stashes", h.Stashes)
	registerWrites(rg, h)
}

func registerWrites(
	rg *gin.RouterGroup,
	h *githandlers.Handlers,
) {
	rg.POST("/workspaces/:wsId/git/stage", h.Stage)
	rg.POST("/workspaces/:wsId/git/unstage", h.Unstage)
	rg.POST("/workspaces/:wsId/git/discard", h.Discard)
	rg.POST("/workspaces/:wsId/git/commit", h.Commit)
	rg.POST("/workspaces/:wsId/git/push", h.Push)
	rg.POST("/workspaces/:wsId/git/pull", h.Pull)
	rg.POST("/workspaces/:wsId/git/fetch", h.Fetch)
	rg.POST("/workspaces/:wsId/git/branches", h.CreateBranch)
	rg.PATCH("/workspaces/:wsId/git/branches/:branch", h.RenameBranch)
	rg.DELETE("/workspaces/:wsId/git/branches/:branch", h.DeleteBranch)
	rg.POST("/workspaces/:wsId/git/checkout", h.Checkout)
	rg.POST("/workspaces/:wsId/git/stash", h.StashPush)
	rg.POST("/workspaces/:wsId/git/stash/:id", h.StashRestore)
	rg.DELETE("/workspaces/:wsId/git/stash/:id", h.StashDrop)
	rg.POST("/workspaces/:wsId/git/reset", h.Reset)
	rg.POST("/workspaces/:wsId/git/merge", h.Merge)
	rg.POST("/workspaces/:wsId/git/rebase", h.Rebase)
}
