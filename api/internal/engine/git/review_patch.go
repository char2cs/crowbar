package git

import (
	"context"
	"io"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

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
