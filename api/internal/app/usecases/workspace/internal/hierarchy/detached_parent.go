package hierarchy

import (
	"context"
	"log/slog"
)

// detachedParentHead is the commit the parent's checkout holds: a branchless
// (detached) parent has nothing else to fork from, and FromParentHead asks for
// it. Unresolved, it answers "" and the worktree add reports why.
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
