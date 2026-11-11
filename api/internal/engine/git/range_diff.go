package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// RangeDiff returns the three-dot diff between base and branch.
func (e *engine) RangeDiff(
	ctx context.Context,
	repoPath string,
	base string,
	branch string,
) (gitdomain.MultiFileDiff, error) {
	return diff.Range(ctx, repoPath, base, branch)
}
