package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

// RemoveUnderHome deletes target only when it is strictly under crowbar home,
// and never fails the caller.
//
// It is exported because the workspace-delete cascade reaps a chat's on-disk
// footprint from the app layer, off the same path resolution and the same guard
// PurgeChat uses — reimplementing either there is how a delete ends up pointed at
// the user's real repository.
func RemoveUnderHome(
	ctx context.Context,
	home string,
	target string,
) {
	worktreepath.RemoveUnderHome(ctx, home, target)
}
