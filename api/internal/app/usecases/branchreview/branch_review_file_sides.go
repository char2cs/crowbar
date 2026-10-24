package branchreview

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// GetFileSides returns one file's full old and new text for the same diff GetPatch
// serves, so the reader can reveal the unchanged lines between hunks. It resolves
// the ref through resolveScopeRef, which keeps both sides on the diff the outline
// laid out.
//
// oldPath names the file on the old side when it was renamed; empty means path.
// A file past the engine's size ceiling is ErrUnprocessable, not a truncated read.
func (u *branchReviewUsecase) GetFileSides(
	ctx context.Context,
	wsID string,
	commit string,
	path string,
	oldPath string,
) (gitdomain.ReviewFileSides, error) {
	if path == "" {
		return gitdomain.ReviewFileSides{}, fmt.Errorf(
			"branch review: file sides: %w: path is required", apperr.ErrInvalidArgument)
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
			"branch review: file sides: %w: %w", apperr.ErrUnprocessable, err)
	}
	if err != nil {
		return gitdomain.ReviewFileSides{}, fmt.Errorf("branch review: file sides: %w", err)
	}
	return sides, nil
}
