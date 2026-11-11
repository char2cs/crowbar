package git

import (
	"context"
	"io"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewFilePatch streams one file's patch against ref into w under the read lock, held
// for the copy so a mutation waits only on one file's patch. It returns the lines written
// and whether maxLines cut it; see diff.FilePatch for the cut and rename semantics.
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
