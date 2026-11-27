package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

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
