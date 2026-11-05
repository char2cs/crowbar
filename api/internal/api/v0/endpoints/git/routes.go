// Package git mounts the v0 git REST routes for a chat's worktree: status,
// diff, log, branches, stashes, conflicts and commit-diff reads, plus staging,
// commit, sync, branch, stash, reset/merge/rebase and operation continue/abort.
package git

import (
	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

// Register mounts /v0/chats/:chatId/git/... . Git state belongs to the
// worktree, so every chat holding it sees the same answer. The slow verbs
// (push, fetch, pull, merge, rebase) run as operations on ops.
func Register(
	chatScoped *gin.RouterGroup,
	gitSvc githandlers.Git,
	ops githandlers.Operations,
) {
	h := githandlers.New(gitSvc, ops)
	mount(chatScoped, "/git", h)
}

// mount registers the git surface under prefix on rg.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *githandlers.Handlers,
) {
	// Reads
	rg.GET(prefix+"/status", h.Status)
	rg.GET(prefix+"/diff", h.Diff)
	rg.GET(prefix+"/log", h.Log)
	rg.GET(prefix+"/branches", h.Branches)
	rg.GET(prefix+"/stashes", h.Stashes)
	rg.GET(prefix+"/conflicts", h.Conflicts)
	rg.GET(prefix+"/conflict-hunks", h.ConflictHunks)
	rg.GET(prefix+"/commit-diff", h.CommitDiff)
	// Writes
	rg.POST(prefix+"/stage", h.Stage)
	rg.POST(prefix+"/stage-hunk", h.StageHunk)
	rg.POST(prefix+"/unstage", h.Unstage)
	rg.POST(prefix+"/unstage-hunk", h.UnstageHunk)
	rg.POST(prefix+"/discard", h.Discard)
	rg.POST(prefix+"/commit", h.Commit)
	rg.POST(prefix+"/push", h.Push)
	rg.POST(prefix+"/fetch", h.Fetch)
	rg.POST(prefix+"/pull", h.Pull)
	rg.POST(prefix+"/branches", h.CreateBranch)
	rg.PATCH(prefix+"/branches", h.RenameBranch)
	rg.DELETE(prefix+"/branches", h.DeleteBranch)
	rg.POST(prefix+"/switch", h.Switch)
	rg.POST(prefix+"/stash", h.StashPush)
	rg.POST(prefix+"/stash-apply", h.StashApply)
	rg.POST(prefix+"/stash-pop", h.StashPop)
	rg.DELETE(prefix+"/stash", h.StashDrop)
	rg.POST(prefix+"/reset", h.Reset)
	rg.POST(prefix+"/merge", h.Merge)
	rg.POST(prefix+"/rebase", h.Rebase)
	rg.POST(prefix+"/resolve-hunk", h.ResolveHunk)
	rg.POST(prefix+"/operation/continue", h.OperationContinue)
	rg.POST(prefix+"/operation/abort", h.OperationAbort)
}
