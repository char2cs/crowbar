package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewFiles returns the files-only diff summary of the working tree against
// ref: the same file set as DiffAgainstRef (committed changes since ref plus
// uncommitted tracked modifications) but with NO line-level content — just the
// per-file status and +/- counts (review §5, Task 27). It runs under the shared
// read lock like every other inspection.
//
// dirty is the set of paths the caller's git status reports as differing from
// HEAD. It keeps the +/- counts off the O(diff size) path: the committed half
// is cached against (repo, ref, HEAD) and only these paths are re-diffed
// against the working tree. Pass nil when no status is available and every
// count is recomputed instead.
func (e *engine) ReviewFiles(
	ctx context.Context,
	repoPath string,
	ref string,
	dirty []string,
) ([]gitdomain.ReviewFileSummary, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.FileSummaries(ctx, repoPath, ref, dirty)
}
