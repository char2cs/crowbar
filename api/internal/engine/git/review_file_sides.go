package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewFileSides reads one file's old and new text for the diff against ref,
// under the shared read lock. See diff.FileSides for which revision each side
// comes from and for the size ceiling.
func (e *engine) ReviewFileSides(
	ctx context.Context,
	repoPath string,
	ref string,
	path string,
	oldPath string,
) (gitdomain.ReviewFileSides, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.FileSides(ctx, repoPath, ref, path, oldPath)
}
