package watch

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// gitStatusEqual reports whether two statuses carry the same broadcastable
// state (branch, ahead/behind, and the exact file list in order).
func gitStatusEqual(
	a gitdomain.GitStatus,
	b gitdomain.GitStatus,
) bool {
	if a.Branch != b.Branch || a.Ahead != b.Ahead || a.Behind != b.Behind {
		return false
	}
	if len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			return false
		}
	}
	return true
}

// scheduleGitRecompute arms (or re-arms) the trailing git-recompute debounce.
// Back-to-back calls collapse into one fanOutGit run, executed by loop once
// changes have been quiet for gitRecomputeQuiet, and never later than
// gitRecomputeMaxWait after the first of them. It is called only from the loop
// goroutine (handleBurst / maybeHandleGitRef), so resetting the timer never
// races with loop's drain of gitTimer.C.
func (w *Watcher) scheduleGitRecompute() {
	w.mu.Lock()
	now := w.now()
	if !w.gitPending {
		w.gitPendingSince = now
	}
	w.gitPending = true
	wait := min(gitRecomputeQuiet, max(0, gitRecomputeMaxWait-now.Sub(w.gitPendingSince)))
	w.mu.Unlock()
	w.gitTimer.Reset(wait)
}

// runGitRecompute clears the pending flag and performs the coalesced git
// recompute. It runs in the loop goroutine when gitTimer fires.
func (w *Watcher) runGitRecompute(
	ctx context.Context,
) {
	w.mu.Lock()
	w.gitPending = false
	w.mu.Unlock()
	w.fanOutGit(ctx)
	if w.onGitRecompute != nil {
		w.onGitRecompute()
	}
}

func (w *Watcher) fanOutGit(
	ctx context.Context,
) {
	if w.isRewriteInProgress() {
		return
	}

	status, err := w.git.ComputeStatus(ctx, w.repoPath)
	if err != nil {
		return
	}
	// Workspaces sharing a .git (linked worktrees) all see each other's ref
	// events; without this guard every such event re-broadcasts an unchanged
	// status to every subscriber (observed as a ~6Hz identical-frame storm).
	// prevStatus starts unset, so the FIRST recompute always broadcasts; since the
	// watcher does no initial recompute on subscribe (see loop — the snapshot
	// already delivers fresh status), that first broadcast is the first real
	// file-change, and subsequent identical frames are deduped here.
	if !w.prevStatusSet || !gitStatusEqual(w.prevStatus, status) {
		w.prevStatus = status
		w.prevStatusSet = true
		w.dispatcher.OnGitStatus(ctx, w.wsID, status)
	}

	if w.diffBase == nil {
		resolved := w.git.ResolveDiffBase(ctx, w.repoPath, w.base)
		w.diffBase = &resolved
	}
	added, deleted, err := w.git.DiffStat(ctx, w.repoPath, w.diffBase.Commit)
	if err != nil {
		return
	}
	hasConflicts := statusHasConflicts(status)
	hasCommits := w.diffBase.HasCommits

	if added == w.prevAdded && deleted == w.prevDeleted &&
		hasConflicts == w.prevConflict && hasCommits == w.prevCommits {
		return
	}

	w.prevAdded = added
	w.prevDeleted = deleted
	w.prevConflict = hasConflicts
	w.prevCommits = hasCommits

	w.dispatcher.OnSyncWorkingTreeState(ctx, SyncInput{
		WsID:         w.wsID,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	})
}

// statusHasConflicts reads the unmerged entries git status already reported, so
// the conflict check costs no process of its own.
func statusHasConflicts(
	status gitdomain.GitStatus,
) bool {
	for _, f := range status.Files {
		if f.Status == gitdomain.GitFileStatusConflicted {
			return true
		}
	}
	return false
}
