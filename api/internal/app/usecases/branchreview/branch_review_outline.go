package branchreview

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// GetOutline returns the hunk geometry of the workspace's branch diff: per
// changed file, the `@@` shapes of its diff against the fork point and no diff
// content whatsoever. It resolves the ref through resolveDiffRef, exactly as
// GetFiles does, so the outline and the file list describe the same diff —
// a different ref here would lay the review out against one diff and populate
// it from another.
//
// The payload is O(hunks) where Get's read model is O(lines): it describes a
// million-line branch in about a megabyte, which is what lets the client
// reserve the right scroll space for every file before fetching a single
// patch. Producing it still costs a full streamed read of the diff, so the
// result is cached whenever the key can be made exact — see
// cacheableOutlineKey.
//
// commit scopes the read to one commit against its parent; empty means the
// branch diff. See resolveScopeRef.
func (u *branchReviewUsecase) GetOutline(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.FileOutline, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return nil, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	key, cacheable := u.cacheableOutlineKey(ctx, ws, ref, commit)
	return u.outlineWithKey(ctx, ws, ref, key, cacheable)
}

// outlineWithKey is the read behind both GetOutline and GetScope's geometry:
// given an already-resolved ref and whatever cache key its caller could prove,
// it serves the memoised outline or streams a fresh one. It is factored out so
// the two callers cannot drift into computing the same geometry two different
// ways — they must agree, or an anchor validated against one would be refused
// against the other.
func (u *branchReviewUsecase) outlineWithKey(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	key outlineKey,
	cacheable bool,
) ([]gitdomain.FileOutline, error) {
	if !cacheable {
		return u.readOutline(ctx, ws, ref)
	}
	return u.outlines.get(key, func() ([]gitdomain.FileOutline, error) {
		return u.readOutline(ctx, ws, ref)
	})
}

func (u *branchReviewUsecase) readOutline(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
) ([]gitdomain.FileOutline, error) {
	files, err := u.git.ReviewOutline(ctx, ws.WorktreePath, ref)
	if err != nil {
		return nil, fmt.Errorf("branch review: review outline: %w", err)
	}
	return files, nil
}

// cacheableOutlineKey reports the (repoPath, ref, headSHA) key under which this
// outline may be memoised, and whether it may be memoised at all.
//
// The branch outline is taken against the WORKING TREE, so it is only a
// function of that key while the working tree has nothing of its own to
// contribute — that is, while `git diff <ref> --` and `git diff <ref> <HEAD> --`
// are the same diff of two immutable trees. A tree that has moved is therefore
// refused a key outright rather than cached under one that cannot see the
// difference, and so is a tree whose state could not be established: an
// unreadable status or an unresolvable HEAD prove nothing, and a cache entry
// may only be written on proof.
//
// A COMMIT-scoped read skips all of that. Its ref names both ends of the diff,
// so it is already a function of two immutable trees and the working tree
// cannot reach it — which means it stays cacheable in exactly the situation the
// branch outline gives up on, a dirty tree, and that is the situation someone
// reading history is most often in.
func (u *branchReviewUsecase) cacheableOutlineKey(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	commit string,
) (outlineKey, bool) {
	if isCommitScoped(commit) {
		return outlineKey{repoPath: ws.WorktreePath, ref: ref}, true
	}
	status, err := u.git.Status(ctx, ws.WorktreePath)
	if err != nil {
		return outlineKey{}, false
	}
	return u.branchOutlineKey(ctx, ws, ref, !hasTrackedChanges(status))
}

// branchOutlineKey is the second half of cacheableOutlineKey, split out for the
// caller that has ALREADY established the working tree's state — GetScope, whose
// file list is read from a status it took on the way — so that fact does not
// cost a second `git status` to re-learn.
//
// treeClean is a precondition, not a hint: a tree that has moved is refused a key
// outright rather than cached under one that cannot see the difference.
func (u *branchReviewUsecase) branchOutlineKey(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	treeClean bool,
) (outlineKey, bool) {
	if !treeClean {
		return outlineKey{}, false
	}
	head, err := u.git.RevParse(ctx, ws.WorktreePath, "HEAD")
	if err != nil || head == "" {
		return outlineKey{}, false
	}
	return outlineKey{repoPath: ws.WorktreePath, ref: ref, headSHA: head}, true
}

// hasTrackedChanges reports whether the working tree differs from HEAD in a way
// the branch diff can see. Untracked files are excluded because `git diff` does
// not report them at all: they cannot change a hunk shape, and counting them
// would disable the cache for any repository carrying so much as a stray
// .DS_Store.
func hasTrackedChanges(
	status gitdomain.GitStatus,
) bool {
	for _, f := range status.Files {
		if f.Status != gitdomain.GitFileStatusUntracked {
			return true
		}
	}
	return false
}
