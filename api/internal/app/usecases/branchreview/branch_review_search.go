package branchreview

import (
	"context"
	"fmt"
	"regexp"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func (u *branchReviewUsecase) SearchDiff(
	ctx context.Context,
	wsID string,
	commit string,
	query string,
	opts gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	if err := validateSearchPattern(query, opts); err != nil {
		return nil, false, err
	}
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return nil, false, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return nil, false, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	hits, truncated, err := u.git.ReviewSearch(ctx, ws.WorktreePath, ref, query, opts)
	if err != nil {
		return nil, false, fmt.Errorf("branch review: review search: %w", err)
	}
	return hits, truncated, nil
}

func validateSearchPattern(
	query string,
	opts gitdomain.SearchOpts,
) error {
	if !opts.Regex {
		return nil
	}
	if _, err := regexp.Compile(query); err != nil {
		return fmt.Errorf("branch review: search: %w: %w", problem.ErrInvalidArgument, err)
	}
	return nil
}
