// Package git mounts all v0 git routes for a worktree: status (dual-served
// with the live WebSocket stream), diff, log, blame, branches, stashes,
// conflicts, conflict-hunks, commit-diff, and all write routes (staging,
// commit, sync, branch, stash, reset/merge/rebase, and operation
// continue/abort).
package git

import (
	"github.com/gin-gonic/gin"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

// Register mounts the git REST and WebSocket surface on BOTH scoping groups it
// is currently addressable through.
//
// git is spec §4.2's shared bucket: the worktree answers once, and every chat
// holding it gets that answer. chatScoped is where that lives from now on —
// /v0/chats/:chatId/git/... , the flat prefix §7.1 closes on — and the
// frontend talks to it exclusively.
//
// wsScoped is the OLD /projects/:projectId/repos/:repoId/workspaces/:wsId/git/...
// surface, mounted unchanged. It is not a fallback and nothing chooses between
// the two: it is simply a route that has not been retired yet, and retiring it
// is spec §8 step 6's job, once every group has moved and the workspaces/home
// groups are deleted wholesale. Deleting THIS call is the whole of git's share
// of that step.
//
// One route table serves both, so the two can never drift into different
// surfaces: mount is called twice with different prefixes, and a route added to
// it appears on both by construction. The handlers themselves take the worktree
// from whichever mount the request arrived on — see handlers.workspaceID.
//
// gitWS is the pre-built broadcaster handle for the live git-status stream; it
// is dual-served on the /git/status route of each prefix (a plain GET answers
// REST, a WebSocket upgrade is routed to gitWS) — the dedicated /ws/git route
// is gone (W7-2).
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	gitSvc githandlers.Git,
	lastErrors githandlers.LastErrorSetter,
	working githandlers.WorkSignal,
	gitWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := githandlers.New(gitSvc, lastErrors, working)
	mount(chatScoped, "/git", h, gitWS, dispatch)
	mount(wsScoped, "/workspaces/:wsId/git", h, gitWS, dispatch)
}

// mount registers the 32-route git surface under prefix on rg. It is the single
// definition of that surface; Register calls it once per live scoping group.
//
//nolint:funlen // Flat route table: one line per route. Splitting it would scatter the surface across helpers for no gain, and this list IS the audited surface (route_audit_test.go).
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *githandlers.Handlers,
	gitWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	// Reads
	rg.GET(prefix+"/status", dispatch(h.Status, gitWS))
	rg.GET(prefix+"/diff", h.Diff)
	rg.GET(prefix+"/log", h.Log)
	rg.GET(prefix+"/blame", h.Blame)
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
