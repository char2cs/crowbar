// Package git mounts all v0 git routes for a workspace: status (dual-served
// with the live WebSocket stream), diff, log, blame, branches, stashes,
// conflicts, conflict-hunks, commit-diff, and all write routes (staging,
// commit, sync, branch, stash, reset/merge/rebase, and operation
// continue/abort).
package git

import (
	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

// Register mounts all git REST and WebSocket routes on rg.
// gitWS is the pre-built broadcaster handle for the live git-status stream;
// it is registered both as the /ws/git topic and as the dual-serve target on
// the /git/status route so clients can upgrade on either URL.
func Register(
	rg *gin.RouterGroup,
	gitSvc githandlers.Git,
	gitWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := githandlers.New(gitSvc)
	// Reads
	rg.GET("/workspaces/:wsId/git/status", dispatch(h.Status, gitWS))
	rg.GET("/workspaces/:wsId/git/diff", h.Diff)
	rg.GET("/workspaces/:wsId/git/log", h.Log)
	rg.GET("/workspaces/:wsId/git/blame", h.Blame)
	rg.GET("/workspaces/:wsId/git/branches", h.Branches)
	rg.GET("/workspaces/:wsId/git/stashes", h.Stashes)
	rg.GET("/workspaces/:wsId/git/conflicts", h.Conflicts)
	rg.GET("/workspaces/:wsId/git/conflict-hunks", h.ConflictHunks)
	rg.GET("/workspaces/:wsId/git/commit-diff", h.CommitDiff)
	// Writes
	rg.POST("/workspaces/:wsId/git/stage", h.Stage)
	rg.POST("/workspaces/:wsId/git/stage-hunk", h.StageHunk)
	rg.POST("/workspaces/:wsId/git/unstage", h.Unstage)
	rg.POST("/workspaces/:wsId/git/unstage-hunk", h.UnstageHunk)
	rg.POST("/workspaces/:wsId/git/discard", h.Discard)
	rg.POST("/workspaces/:wsId/git/commit", h.Commit)
	rg.POST("/workspaces/:wsId/git/push", h.Push)
	rg.POST("/workspaces/:wsId/git/fetch", h.Fetch)
	rg.POST("/workspaces/:wsId/git/pull", h.Pull)
	rg.POST("/workspaces/:wsId/git/branches", h.CreateBranch)
	rg.PATCH("/workspaces/:wsId/git/branches", h.RenameBranch)
	rg.DELETE("/workspaces/:wsId/git/branches", h.DeleteBranch)
	rg.POST("/workspaces/:wsId/git/switch", h.Switch)
	rg.POST("/workspaces/:wsId/git/stash", h.StashPush)
	rg.POST("/workspaces/:wsId/git/stash-apply", h.StashApply)
	rg.POST("/workspaces/:wsId/git/stash-pop", h.StashPop)
	rg.DELETE("/workspaces/:wsId/git/stash", h.StashDrop)
	rg.POST("/workspaces/:wsId/git/reset", h.Reset)
	rg.POST("/workspaces/:wsId/git/merge", h.Merge)
	rg.POST("/workspaces/:wsId/git/rebase", h.Rebase)
	rg.POST("/workspaces/:wsId/git/resolve-hunk", h.ResolveHunk)
	rg.POST("/workspaces/:wsId/git/operation/continue", h.OperationContinue)
	rg.POST("/workspaces/:wsId/git/operation/abort", h.OperationAbort)
	// WebSocket — co-located with HTTP git routes
	rg.GET("/ws/git", gitWS)
}
