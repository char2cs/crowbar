package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// ReviewOutline returns the `@@` hunk shapes of ReviewFiles' set, under the read lock.
// It streams the diff and keeps only headers, so memory is O(hunks), not O(diff).
func (e *engine) ReviewOutline(
	ctx context.Context,
	repoPath string,
	ref string,
) ([]gitdomain.FileOutline, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.Outline(ctx, repoPath, ref)
}
