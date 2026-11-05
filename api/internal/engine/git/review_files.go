package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewFiles returns DiffAgainstRef's file set with status and +/- counts but no hunks,
// under the read lock. dirty (paths git status reports changed) keeps it off the O(diff)
// path: the committed half is cached per (repo, ref, HEAD); nil recomputes every count.
func (e *engine) ReviewFiles(
	ctx context.Context,
	repoPath string,
	ref string,
	dirty []string,
) ([]gitdomain.ReviewFileSummary, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.FileSummaries(ctx, repoPath, ref, dirty)
}
