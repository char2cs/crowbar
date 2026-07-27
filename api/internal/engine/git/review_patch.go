package git

import (
	"context"
	"io"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewFilePatch streams one file's unified patch against ref into w, under
// the shared read lock like every other inspection. The lock is held for the
// whole copy, which is what a read lock is for: concurrent inspections proceed,
// and a mutation waits for a read that is bounded by the size of ONE file's
// patch rather than the branch's.
//
// It returns the number of patch lines written and whether the patch was cut
// short at maxLines. See diff.FilePatch for the cut's hunk-boundary guarantee
// and for why a renamed file addressed by its new path reads as an addition.
func (e *engine) ReviewFilePatch(
	ctx context.Context,
	repoPath string,
	ref string,
	path string,
	maxLines int,
	w io.Writer,
) (int, bool, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.FilePatch(ctx, repoPath, ref, path, maxLines, w)
}
