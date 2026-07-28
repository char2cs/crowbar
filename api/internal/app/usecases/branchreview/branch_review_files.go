package branchreview

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/singleflight"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// GetFiles returns the files-only branch-review summary for a workspace. It
// diffs the workspace's fork point against the working tree for the file set +
// counts (git.ReviewFiles — name-status + numstat, no content), then folds in
// git status so working-tree changes are flagged uncommitted/staged and plain
// untracked files (which have no diff against the fork point) still surface. The
// payload is O(file count), so the sidebar can render the full changed-files
// list with +N/-N badges without ever fetching the line-level branch diff.
//
// Concurrent calls for one workspace are single-flighted: the sidebar debounces
// its refetch, but a burst still overlaps, and each overlapping call would
// otherwise take the repo read lock and re-run the whole merge-base/status/diff
// sequence. Sharing beats cancelling the loser — every caller still gets a
// correct, current answer. This is deduplication of in-flight work ONLY, never
// a cache: a finished flight is dropped, so the next call recomputes rather
// than serving a stale file list.
// commit scopes the read to one commit against its parent; empty means the
// branch diff. See resolveScopeRef. It is part of the single-flight key: two
// callers asking about different diffs are not the same flight.
func (u *branchReviewUsecase) GetFiles(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.ReviewFileSummary, error) {
	shared := u.fileReads.DoChan(wsID+"\x00"+commit, func() (any, error) {
		return u.readFiles(context.WithoutCancel(ctx), wsID, commit)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-shared:
		return sharedFiles(res)
	}
}

// sharedFiles unwraps a flight result for one waiter. The slice is cloned per
// waiter because the flight hands the same backing array to every one of them,
// and ReviewFileSummary is a flat value struct, so a shallow clone fully
// isolates callers.
func sharedFiles(
	res singleflight.Result,
) ([]gitdomain.ReviewFileSummary, error) {
	if res.Err != nil {
		return nil, res.Err
	}
	files, _ := res.Val.([]gitdomain.ReviewFileSummary)
	return slices.Clone(files), nil
}

// readFiles is the shared computation behind GetFiles. It runs under a context
// detached from cancellation: the flight is owned by whichever caller happened
// to start it, and that caller disconnecting must not fail the waiters behind
// it. The work stays bounded by exec.GitOpTimeout on every git invocation.
func (u *branchReviewUsecase) readFiles(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.ReviewFileSummary, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return nil, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	// A commit-scoped read has nothing to do with the working tree: its ref names
	// both ends, so no file it lists is "uncommitted" and no count of its can be
	// stale. Skipping the status is not just an optimisation — merging one in
	// would flag files as dirty because the tree happens to be dirty NOW, in a
	// diff of two commits that closed long ago.
	if isCommitScoped(commit) {
		files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, nil)
		if err != nil {
			return nil, fmt.Errorf("branch review: review files: %w", err)
		}
		return files, nil
	}
	// The status is fetched BEFORE the summary and threaded into it: its paths
	// are what let ReviewFiles keep the +/- counts off the O(diff size) path,
	// recomputing only what the working tree has moved and taking the rest from
	// its committed-diff cache. This is not an extra call — it is the one
	// mergeWorkingTree already made for the uncommitted/staged flags, moved
	// ahead of the summary so a single status serves both.
	status, statusErr := u.git.Status(ctx, ws.WorktreePath)
	files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, dirtyPaths(status, statusErr))
	if err != nil {
		return nil, fmt.Errorf("branch review: review files: %w", err)
	}
	if statusErr != nil {
		return files, nil
	}
	return mergeWorkingTree(status, files), nil
}

// dirtyPaths lists every path the working-tree status reports. A status failure
// yields nil, which tells ReviewFiles the dirty set is unknown so it recomputes
// every count against the working tree instead of trusting a committed one.
func dirtyPaths(
	status gitdomain.GitStatus,
	err error,
) []string {
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(status.Files))
	for _, f := range status.Files {
		out = append(out, f.Path)
	}
	return out
}

// mergeWorkingTree annotates each committed/tracked summary entry with its
// working-tree state and appends untracked files. A status failure is
// non-fatal — the committed picture is still returned, just without the flags —
// mirroring annotateUncommitted.
func mergeWorkingTree(
	status gitdomain.GitStatus,
	files []gitdomain.ReviewFileSummary,
) []gitdomain.ReviewFileSummary {
	dirty, staged := workingTreeIndex(status)
	for i := range files {
		files[i].Uncommitted = dirty[files[i].Path]
		files[i].Staged = staged[files[i].Path]
	}
	return append(files, untrackedSummaries(status, files)...)
}

func workingTreeIndex(
	status gitdomain.GitStatus,
) (map[string]bool, map[string]bool) {
	dirty := make(map[string]bool, len(status.Files))
	staged := make(map[string]bool, len(status.Files))
	for _, f := range status.Files {
		dirty[f.Path] = true
		staged[f.Path] = staged[f.Path] || f.Staged
	}
	return dirty, staged
}

func untrackedSummaries(
	status gitdomain.GitStatus,
	existing []gitdomain.ReviewFileSummary,
) []gitdomain.ReviewFileSummary {
	present := make(map[string]bool, len(existing))
	for _, f := range existing {
		present[f.Path] = true
	}
	var out []gitdomain.ReviewFileSummary
	for _, f := range status.Files {
		if !isNewUntracked(f, present) {
			continue
		}
		present[f.Path] = true
		out = append(out, gitdomain.ReviewFileSummary{
			Path:        f.Path,
			Status:      gitdomain.GitFileStatusUntracked,
			Uncommitted: true,
		})
	}
	return out
}

func isNewUntracked(
	f gitdomain.GitFile,
	present map[string]bool,
) bool {
	return f.Status == gitdomain.GitFileStatusUntracked && !present[f.Path]
}
