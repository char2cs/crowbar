package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
)

// SpawnChatWithOwnWorktree fills chatID's empty workspace slot with a fresh
// worktree forked from its resolved fork parent — the SAME WorktreeCreator port
// Promote uses (model spec §4.2, promote.go) — and starts providerID's CLI in
// it.
//
// It is CreateChat's ownWorktree counterpart (chat/internal/tree/chats.go):
// chatID has just been minted and placed, so its lineage (and here, the fork
// parent its own walk resolves) is fixed before this runs, but it has never had
// a runner. That is the one difference from Promote, which fills an EXISTING
// bubble's slot and must tear its live CLI down and respawn via SwitchProvider
// (model spec §4.2 step 3); a chat with no runner yet only needs a first,
// ordinary StartRunner once its workspace exists.
//
// It resolves the fork parent through tree.ResolveForkParentFresh, NOT
// Promote's plain tree.ResolveForkParent: chatID was placed microseconds ago in
// this SAME call, so the fork-parent walk needs the log-corrected read or it
// can see chatID's own row exactly as it stood before that placement — see
// ResolveForkParentFresh's own doc.
//
// A failure after the workspace is minted takes it back out — unpromote's own
// rollback shape, reused verbatim — because a chat must never observably own a
// workspace with no CLI in it. The chat row itself is left for the caller to
// discard: CreateChat's own contract, mirroring the thread path's discard on a
// StartRunner failure, is that a create the user was told failed leaves no
// chat behind.
func (u *Usecase) SpawnChatWithOwnWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	forkParentID, ok, err := tree.ResolveForkParentFresh(ctx, u.chats, chatID)
	if err != nil {
		return "", fmt.Errorf("create chat: resolve fork parent: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("create chat %s: %w", chatID, ErrNoForkParent)
	}
	ws, err := u.worktree.CreateChildWorkspace(ctx, forkParentID)
	if err != nil {
		return "", fmt.Errorf("create chat: create workspace: %w", err)
	}
	if _, err := u.chats.SetWorkspace(ctx, chatID, ws.ID); err != nil {
		return "", u.unpromote(ctx, chatID, ws.ID, false,
			fmt.Errorf("create chat: set workspace: %w", err))
	}
	runnerID, err := u.runners.StartRunner(ctx, chatID, providerID)
	if err != nil {
		return "", u.unpromote(ctx, chatID, ws.ID, true,
			fmt.Errorf("create chat: start runner: %w", err))
	}
	return runnerID, nil
}
