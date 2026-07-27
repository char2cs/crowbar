package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewOutline returns the hunk geometry of ReviewFiles' file set: per file,
// the `@@` shapes of its diff against ref and nothing else. It streams the diff
// and keeps only the headers, so the daemon's memory is O(hunks) — a
// million-line diff is described in about a megabyte — where DiffAgainstRef
// would hold the whole of it. Runs under the shared read lock like every other
// inspection.
func (e *engine) ReviewOutline(
	ctx context.Context,
	repoPath string,
	ref string,
) ([]gitdomain.FileOutline, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.Outline(ctx, repoPath, ref)
}
