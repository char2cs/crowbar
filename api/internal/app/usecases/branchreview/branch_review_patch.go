package branchreview

import (
	"context"
	"fmt"
	"io"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// GetPatch streams ONE file's unified patch from the workspace's branch diff
// into w, and reports how many patch lines it wrote and whether the patch was
// cut short at maxLines. It resolves the ref through resolveDiffRef, exactly as
// GetFiles and GetOutline do, so the patch a reader opens belongs to the same
// diff the outline laid out.
//
// path addresses the file by its NEW name and the bytes go straight from git's
// stdout to w: neither this usecase nor the engine beneath it ever holds the
// patch, which is what lets a 420,000-line file be served without a
// diff-sized allocation anywhere. maxLines <= 0 means unlimited; a positive cap
// always cuts on a hunk boundary so what reaches the client is a patch its
// parser can read.
//
// An empty path is refused before a subprocess exists. The engine repeats the
// guard (diff.ErrEmptyPatchPath) because it is not mere input validation:
// ":(top,literal)" with nothing appended is a valid pathspec matching the top
// of the tree, so an empty path would quietly turn the one query guaranteed to
// be O(one file) into a read of the entire branch diff.
// commit scopes the read to one commit against its parent; empty means the
// branch diff. See resolveScopeRef.
func (u *branchReviewUsecase) GetPatch(
	ctx context.Context,
	wsID string,
	commit string,
	path string,
	maxLines int,
	w io.Writer,
) (int, bool, error) {
	if path == "" {
		return 0, false, fmt.Errorf("branch review: patch: %w: path is required", apperr.ErrInvalidArgument)
	}
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return 0, false, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return 0, false, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	lines, truncated, err := u.git.ReviewFilePatch(ctx, ws.WorktreePath, ref, path, maxLines, w)
	if err != nil {
		return lines, truncated, fmt.Errorf("branch review: file patch: %w", err)
	}
	return lines, truncated, nil
}
