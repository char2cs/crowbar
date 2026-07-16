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
// read lock like every other inspection, and shells out only to name-status +
// numstat, so it is cheap and O(file count) regardless of diff size.
func (e *engine) ReviewFiles(
	ctx context.Context,
	repoPath string,
	ref string,
) ([]gitdomain.ReviewFileSummary, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.FileSummaries(ctx, repoPath, ref)
}
