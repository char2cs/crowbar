package branchreview

import (
	"context"
	"fmt"
	"io"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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
