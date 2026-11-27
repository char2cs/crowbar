package git

import (
	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

func Register(
	chatScoped *gin.RouterGroup,
	gitSvc githandlers.Git,
	ops githandlers.Operations,
) {
	h := githandlers.New(gitSvc, ops)
	mount(chatScoped, "/git", h)
}

func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *githandlers.Handlers,
) {
	rg.GET(prefix+"/status", h.Status)
	rg.GET(prefix+"/diff", h.Diff)
	rg.GET(prefix+"/log", h.Log)
	rg.GET(prefix+"/branches", h.Branches)
	rg.GET(prefix+"/stashes", h.Stashes)
	rg.GET(prefix+"/conflicts", h.Conflicts)
	rg.GET(prefix+"/conflict-hunks", h.ConflictHunks)
	rg.GET(prefix+"/commit-diff", h.CommitDiff)
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
