package branchreview

import (
	"context"
	"fmt"
	"io"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// GetPatch streams ONE file's patch (by NEW path) from git straight into w, never holding it, and reports
// lines written and whether maxLines (<= 0 unlimited, cut on a hunk boundary) truncated it. An empty path
// is refused: it would match the whole tree. commit scopes the read; see resolveScopeRef.
func (u *branchReviewUsecase) GetPatch(
	ctx context.Context,
	wsID string,
	commit string,
	path string,
	maxLines int,
	w io.Writer,
) (int, bool, error) {
	if path == "" {
		return 0, false, fmt.Errorf("branch review: patch: %w: path is required", problem.ErrInvalidArgument)
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
