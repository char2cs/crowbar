package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// DiffAgainstRef returns the working-tree-inclusive diff against ref (review §5).
func (e *engine) DiffAgainstRef(
	ctx context.Context,
	repoPath string,
	ref string,
) (gitdomain.MultiFileDiff, error) {
	return diff.AgainstRef(ctx, repoPath, ref)
}
