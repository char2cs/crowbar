package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

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
