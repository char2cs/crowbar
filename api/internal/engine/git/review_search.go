package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewSearch finds query in the diff against ref (ReviewFiles' blend) and returns one
// hit per matching line, plus whether opts.Limit cut them short. It streams the diff, so
// memory is O(limit); runs under the read lock.
func (e *engine) ReviewSearch(
	ctx context.Context,
	repoPath string,
	ref string,
	query string,
	opts gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.SearchDiff(ctx, repoPath, ref, query, opts)
}
