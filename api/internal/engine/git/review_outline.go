package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func (e *engine) ReviewOutline(
	ctx context.Context,
	repoPath string,
	ref string,
) ([]gitdomain.FileOutline, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.Outline(ctx, repoPath, ref)
}
