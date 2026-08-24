package chat

import (
	"context"
	"log/slog"
	"os"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// RemoveUnderHome deletes target only when it is strictly under crowbar home,
// and never fails the caller. Every agent path this package reaps is derived
// from a workspace lookup, so the guard is what stops a poisoned or
// misconfigured chats dir reaching the user's real repository.
func RemoveUnderHome(
	ctx context.Context,
	home string,
	target string,
) {
	if !worktreepath.UnderHome(target, home) {
		slog.WarnContext(ctx, "agent: refusing to rm agent path outside crowbar home (skipping)",
			"target", target, "home", home)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		slog.WarnContext(ctx, "agent: reap agent path", "target", target, "err", err)
	}
}
