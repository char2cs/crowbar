package watch

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

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
		w.reportUnreadable(ctx, err)
		return
	}
	w.unreadable = false
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

func (w *Watcher) reportUnreadable(
	ctx context.Context,
	err error,
) {
	w.prevStatusSet = false
	w.diffBase = nil
	if w.unreadable || ctx.Err() != nil {
		return
	}
	w.unreadable = true
	w.dispatcher.OnGitStatusUnreadable(ctx, w.wsID, err)
}

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
