package hierarchy

import (
	"context"
	"log/slog"
)

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
