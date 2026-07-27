package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewSearch finds query in the content of the diff against ref — the same
// blend of committed and uncommitted changes ReviewFiles describes — and
// returns one hit per matching line with the file and line number to navigate
// to, plus whether opts.Limit cut the results short. It streams the diff and
// keeps only the hits, so the daemon's memory is O(limit) rather than O(diff
// size). Runs under the shared read lock like every other inspection.
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
