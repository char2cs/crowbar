package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func (e *engine) ReviewFiles(
	ctx context.Context,
	repoPath string,
	ref string,
	dirty []string,
) ([]gitdomain.ReviewFileSummary, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.FileSummaries(ctx, repoPath, ref, dirty)
}
