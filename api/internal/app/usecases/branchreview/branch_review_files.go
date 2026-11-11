package branchreview

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/singleflight"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// GetFiles returns the files-only review summary (name-status + numstat folded with git status), O(file
// count) so the sidebar never fetches line diffs. Calls are single-flighted per (workspace, commit) to
// dedupe bursts — never cached. commit scopes the read to one commit; empty means the branch diff.
func (u *branchReviewUsecase) GetFiles(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.ReviewFileSummary, error) {
	read, err := u.scopeOf(ctx, wsID, commit, domain.Workspace{})
	if err != nil {
		return nil, err
	}
	return read.scope.Files, nil
}

// GetScope returns the review ref, its changed files and their hunk geometry from one ref resolution,
// sharing GetFiles' flight. Geometry loads outside the flight off the resolved ref so it describes the
// same diff and GetFiles stays cheap; ws saves a second fold of the aggregate.
func (u *branchReviewUsecase) GetScope(
	ctx context.Context,
	ws domain.Workspace,
) (gitdomain.ReviewScope, error) {
	read, err := u.scopeOf(ctx, ws.ID, "", ws)
	if err != nil {
		return gitdomain.ReviewScope{}, err
	}
	outline, err := u.outlineForScope(ctx, ws, read)
	if err != nil {
		return gitdomain.ReviewScope{}, err
	}
	read.scope.Outline = outline
	return read.scope, nil
}

// outlineForScope loads the geometry of an already-resolved scope, reusing its ref and working-tree
// state instead of going back to git.
func (u *branchReviewUsecase) outlineForScope(
	ctx context.Context,
	ws domain.Workspace,
	read scopeRead,
) ([]gitdomain.FileOutline, error) {
	key, cacheable := u.branchOutlineKey(ctx, ws, read.scope.Base, read.treeClean)
	return u.outlineWithKey(ctx, ws, read.scope.Base, key, cacheable)
}

// scopeRead is one flight's answer plus whether the tree was clean, which the outline cache key needs
// and the status call already learned. A commit-scoped read takes no status and reports false: no
// proof of clean means no cache key.
type scopeRead struct {
	scope     gitdomain.ReviewScope
	treeClean bool
}

// scopeOf is the single-flighted read behind GetFiles and GetScope. known is the caller's resolved
// workspace or zero; resolution happens inside the flight so concurrent readers share one read.
func (u *branchReviewUsecase) scopeOf(
	ctx context.Context,
	wsID string,
	commit string,
	known domain.Workspace,
) (scopeRead, error) {
	shared := u.fileReads.DoChan(wsID+"\x00"+commit, func() (any, error) {
		return u.readScope(context.WithoutCancel(ctx), wsID, commit, known)
	})
	select {
	case <-ctx.Done():
		return scopeRead{}, ctx.Err()
	case res := <-shared:
		return sharedScope(res)
	}
}

// sharedScope unwraps a flight result for one waiter, cloning the file slice because every waiter
// gets the same backing array (ReviewFileSummary is flat, so a shallow clone isolates).
func sharedScope(
	res singleflight.Result,
) (scopeRead, error) {
	if res.Err != nil {
		return scopeRead{}, res.Err
	}
	read, _ := res.Val.(scopeRead)
	read.scope.Files = slices.Clone(read.scope.Files)
	return read, nil
}

// readScope runs under a context detached from cancellation so the flight's starter disconnecting
// cannot fail its waiters; every git call stays bounded by exec.GitOpTimeout.
func (u *branchReviewUsecase) readScope(
	ctx context.Context,
	wsID string,
	commit string,
	known domain.Workspace,
) (scopeRead, error) {
	ws, err := u.workspaceFor(ctx, wsID, known)
	if err != nil {
		return scopeRead{}, err
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return scopeRead{}, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	files, clean, err := u.filesForRef(ctx, ws, ref, commit)
	if err != nil {
		return scopeRead{}, err
	}
	return scopeRead{scope: gitdomain.ReviewScope{Base: ref, Files: files}, treeClean: clean}, nil
}

// workspaceFor returns the workspace to read, folding it from the event log unless known is that
// very workspace — the id guard stops a wrong or zero aggregate producing another workspace's review.
func (u *branchReviewUsecase) workspaceFor(
	ctx context.Context,
	wsID string,
	known domain.Workspace,
) (domain.Workspace, error) {
	if known.ID != "" && known.ID == wsID {
		return known, nil
	}
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	return ws, nil
}

// filesForRef reads the changed-file summary of an ALREADY-resolved ref, so the
// resolution can be reused by whoever also wants the ref itself. It also reports
// whether the status it took proves the working tree clean — see scopeRead.
func (u *branchReviewUsecase) filesForRef(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	commit string,
) ([]gitdomain.ReviewFileSummary, bool, error) {
	// A commit-scoped read names both ends, so merging status would wrongly flag files dirty because
	// the tree is dirty now.
	if isCommitScoped(commit) {
		files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, nil)
		if err != nil {
			return nil, false, fmt.Errorf("branch review: review files: %w", err)
		}
		return files, false, nil
	}
	// Status is fetched before the summary so ReviewFiles recomputes counts only for paths the working
	// tree moved; the same status serves mergeWorkingTree's flags.
	status, statusErr := u.git.Status(ctx, ws.WorktreePath)
	files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, dirtyPaths(status, statusErr))
	if err != nil {
		return nil, false, fmt.Errorf("branch review: review files: %w", err)
	}
	// A status that FAILED proves nothing about the tree, so it reports not-clean
	// for the same reason cacheableOutlineKey refuses a key on one: a cache entry
	// may only be written on proof.
	clean := statusErr == nil && !hasTrackedChanges(status)
	if statusErr != nil {
		return files, clean, nil
	}
	return mergeWorkingTree(status, files), clean, nil
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

// mergeWorkingTree annotates summary entries with working-tree state and appends untracked files. A
// status failure is non-fatal: the committed picture is returned without flags.
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
