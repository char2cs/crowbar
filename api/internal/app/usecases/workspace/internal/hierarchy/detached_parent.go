package hierarchy

import (
	"context"
	"log/slog"
)

// detachedParentHead is the commit a branchless (detached) parent's checkout
// holds: there is no branch to fork from, only that commit. Unresolved, it
// answers "" and the worktree add reports why.
func (u *hierarchyUsecase) detachedParentHead(
	ctx context.Context,
	in CreateChildInput,
) string {
	if in.parentWorktree == "" {
		return ""
	}
	head, err := u.git.RevParse(ctx, in.parentWorktree, "HEAD")
	if err != nil {
		slog.WarnContext(ctx, "create child: detached parent HEAD unresolved",
			"worktree", in.parentWorktree, "err", err)
		return ""
	}
	return head
}
