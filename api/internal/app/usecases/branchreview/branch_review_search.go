package branchreview

import (
	"context"
	"fmt"
	"regexp"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SearchDiff finds query in the CONTENT of the workspace's branch diff and
// returns the file and line number of every match, plus whether opts.Limit cut
// the results short. It resolves the ref through resolveDiffRef, exactly as
// GetFiles and GetOutline do, so a hit's file and line number address the same
// diff the reader is looking at.
//
// It is the server-side replacement for find-in-diff: once the client only ever
// holds a window of the patch, it has nothing left to search locally. The diff
// is streamed and matched a line at a time and the scan stops the moment the
// limit fills, so the cost is bounded by the hits asked for rather than by the
// size of the diff.
func (u *branchReviewUsecase) SearchDiff(
	ctx context.Context,
	wsID string,
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
	ref, err := u.resolveDiffRef(ctx, ws)
	if err != nil {
		return nil, false, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	hits, truncated, err := u.git.ReviewSearch(ctx, ws.WorktreePath, ref, query, opts)
	if err != nil {
		return nil, false, fmt.Errorf("branch review: review search: %w", err)
	}
	return hits, truncated, nil
}

// validateSearchPattern rejects a broken regex before any git work happens. A
// find-as-you-type box submits every half-finished pattern the user passes
// through, so "a(b" is the client's mistake and must surface as an invalid
// argument rather than as a daemon failure — and it must not cost a subprocess
// to discover. Only regex mode can fail: literal mode quotes the query before
// compiling it, which is why the same string is perfectly valid there.
func validateSearchPattern(
	query string,
	opts gitdomain.SearchOpts,
) error {
	if !opts.Regex {
		return nil
	}
	if _, err := regexp.Compile(query); err != nil {
		return fmt.Errorf("branch review: search: %w: %w", apperr.ErrInvalidArgument, err)
	}
	return nil
}
