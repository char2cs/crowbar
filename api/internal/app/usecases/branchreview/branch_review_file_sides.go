package branchreview

import (
	"context"
	"errors"
	"fmt"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// GetFileSides returns one file's full old and new text on the diff GetPatch serves, so
// the reader can reveal lines between hunks. oldPath is the pre-rename name ("" = path);
// a file past the engine's size ceiling is ErrUnprocessable, not a truncated read.
func (u *branchReviewUsecase) GetFileSides(
	ctx context.Context,
	wsID string,
	commit string,
	path string,
	oldPath string,
) (gitdomain.ReviewFileSides, error) {
	if path == "" {
		return gitdomain.ReviewFileSides{}, fmt.Errorf(
			"branch review: file sides: %w: path is required", problem.ErrInvalidArgument)
	}
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return gitdomain.ReviewFileSides{}, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return gitdomain.ReviewFileSides{}, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	sides, err := u.git.ReviewFileSides(ctx, ws.WorktreePath, ref, path, oldPath)
	if errors.Is(err, enginegit.ErrFileTooLarge) {
		return gitdomain.ReviewFileSides{}, fmt.Errorf(
			"branch review: file sides: %w: %w", problem.ErrUnprocessable, err)
	}
	if err != nil {
		return gitdomain.ReviewFileSides{}, fmt.Errorf("branch review: file sides: %w", err)
	}
	return sides, nil
}
