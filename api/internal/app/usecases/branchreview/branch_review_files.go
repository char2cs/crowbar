package branchreview

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/singleflight"

	"github.com/char2cs/crowbar/api/internal/domain"
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
	read, err := u.scopeOf(ctx, wsID, commit, domain.Workspace{})
	if err != nil {
		return nil, err
	}
	return read.scope.Files, nil
}

// GetScope returns the ref the review diffs against, the files that differ from
// it, and the hunk geometry of those differences, from ONE ref resolution. The
// ref and the file list were always computed together — the ref was simply
// thrown away — so that pair costs exactly what the file list alone used to, and
// shares its flight: a sidebar refresh and an agent asking what it is reviewing
// now collapse into one sequence of git calls instead of two.
//
// The geometry is the one part that is genuinely extra work: a streamed read of
// the diff. It is loaded HERE rather than left to the caller's own GetOutline
// because doing it there would resolve the ref a second time and fold the
// workspace aggregate again — the exact duplication this method exists to
// remove — and because geometry fetched separately can describe a different
// diff than the file list it is read beside. It is loaded OUTSIDE the flight,
// off the already-resolved ref, so GetFiles — the sidebar's read, which wants no
// geometry — keeps costing what it costs today.
//
// ws is the caller's already-resolved workspace and saves the second fold of the
// same aggregate — see the interface doc for why that is worth a signature that
// differs from every sibling method's.
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

// outlineForScope loads the geometry of an ALREADY-resolved scope, reusing the
// ref that scope was diffed from and the working-tree state its own status call
// already established. Both are what a bare GetOutline would have gone back to
// git for.
func (u *branchReviewUsecase) outlineForScope(
	ctx context.Context,
	ws domain.Workspace,
	read scopeRead,
) ([]gitdomain.FileOutline, error) {
	key, cacheable := u.branchOutlineKey(ctx, ws, read.scope.Base, read.treeClean)
	return u.outlineWithKey(ctx, ws, read.scope.Base, key, cacheable)
}

// scopeRead is one flight's answer: the scope itself plus whether the working
// tree was clean when it was read.
//
// treeClean travels with the scope because the status that establishes it is
// already taken on the way to the file list, and it is exactly the precondition
// an outline cache key needs (see cacheableOutlineKey). Recomputing it outside
// would be a second `git status` — one subprocess — to learn something this read
// already knew.
//
// A COMMIT-scoped read takes no status at all and reports false. That is not a
// claim the tree is dirty; it is the absence of proof that it is clean, and only
// proof may produce a cache key. Nothing loses by it: GetScope is the only
// consumer of this field and never asks for a commit-scoped read.
type scopeRead struct {
	scope     gitdomain.ReviewScope
	treeClean bool
}

// scopeOf is the single-flighted read behind both GetFiles and GetScope. known
// is the caller's already-resolved workspace, or the zero value when the caller
// holds only an id; either way the resolution happens INSIDE the flight, so
// concurrent readers of the same workspace still share one workspace read rather
// than each folding the aggregate before they meet.
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

// sharedScope unwraps a flight result for one waiter. The file slice is cloned
// per waiter because the flight hands the same backing array to every one of
// them, and ReviewFileSummary is a flat value struct, so a shallow clone fully
// isolates callers. Base is a string and needs no such care.
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

// readScope is the shared computation behind the flight. It runs under a context
// detached from cancellation: the flight is owned by whichever caller happened
// to start it, and that caller disconnecting must not fail the waiters behind
// it. The work stays bounded by exec.GitOpTimeout on every git invocation.
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

// workspaceFor returns the workspace to read the scope of, folding it from the
// event log only when the caller did not already hold it.
//
// The id guard is what makes handing an aggregate in safe: known is only trusted
// when it IS the workspace being asked about, so a caller that passes the wrong
// one — or the zero value — falls through to the real read instead of silently
// producing another workspace's review.
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
	// A commit-scoped read has nothing to do with the working tree: its ref names
	// both ends, so no file it lists is "uncommitted" and no count of its can be
	// stale. Skipping the status is not just an optimisation — merging one in
	// would flag files as dirty because the tree happens to be dirty NOW, in a
	// diff of two commits that closed long ago.
	if isCommitScoped(commit) {
		files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, nil)
		if err != nil {
			return nil, false, fmt.Errorf("branch review: review files: %w", err)
		}
		return files, false, nil
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
