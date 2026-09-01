package chat

import (
	"context"
	"fmt"
	"log/slog"

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
// The fork parent is resolved through tree.ResolveForkParent, log-corrected
// against chatID's own just-written placement (see its own doc, walk.go) —
// chatID was placed microseconds ago in THIS SAME call, so a plain list read
// could still see it exactly as it stood before that placement.
//
// A failure after the workspace is minted takes it back out. This does NOT
// reuse Promote's own unpromote (promote.go): unpromote is safe there because
// a failed PROMOTION leaves chatID surviving as the bubble it was, so a slot
// that fails to clear must leave the still-claimed workspace alone. Here
// chatID is a chat the caller (CreateChat's ownWorktree branch,
// chat/internal/tree/chats.go) is ABOUT TO PURGE OUTRIGHT on any error this
// method returns — there is no surviving row left to claim the workspace once
// that happens, so leaving it behind on a failed clear would orphan it
// instead of protecting it. See discardMintedWorkspace's own doc.
func (u *Usecase) SpawnChatWithOwnWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	forkParentID, ok, err := tree.ResolveForkParent(ctx, u.chats, chatID)
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
		return "", u.discardUnclaimedWorkspace(ctx, ws.ID,
			fmt.Errorf("create chat: set workspace: %w", err))
	}
	runnerID, err := u.runners.StartRunner(ctx, chatID, providerID)
	if err != nil {
		return "", u.discardMintedWorkspace(ctx, chatID, ws.ID,
			fmt.Errorf("create chat: start runner: %w", err))
	}
	return runnerID, nil
}

// discardUnclaimedWorkspace takes back a workspace CreateChildWorkspace just
// minted when SetWorkspace itself failed — chatID's row was never actually
// pointed at ws, so there is nothing to clear first, only the workspace to
// discard. Best-effort and logged, never returned: the caller is told the
// failure that actually happened (cause).
func (u *Usecase) discardUnclaimedWorkspace(
	ctx context.Context,
	workspaceID string,
	cause error,
) error {
	if err := u.worktree.DiscardChildWorkspace(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "agent: create chat: discard the workspace nothing came to own",
			"workspace_id", workspaceID, "err", err)
	}
	return cause
}

// discardMintedWorkspace takes back a workspace CreateChildWorkspace minted
// AFTER chatID's row was already pointed at it (StartRunner failed) — the
// point in Promote's own unpromote (promote.go) that deliberately KEEPS the
// workspace when the clear write fails, because Promote's chat survives and
// still names it.
//
// It must not do that here. chatID does NOT survive a failure from
// SpawnChatWithOwnWorktree: CreateChat's ownWorktree branch purges it outright
// on any error this method returns (the same "a create the user was told
// failed must not leave a chat behind" contract the plain thread path already
// gives its own StartRunner failure). Once that purge runs, a workspace left
// behind because "a row still points at it" is no longer protected by
// anything — it is simply unreachable, since the only way to reach a
// workspace is through the row that owns it. So the clear is attempted,
// best-effort, only to close the narrow window before that purge where a
// concurrent read could otherwise see the row pointing at a workspace that is
// about to be gone — and the discard runs UNCONDITIONALLY regardless of
// whether the clear succeeded.
func (u *Usecase) discardMintedWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
	cause error,
) error {
	if _, err := u.chats.SetWorkspace(ctx, chatID, ""); err != nil {
		slog.WarnContext(ctx, "agent: create chat: clear the workspace slot before discard (best-effort, continuing)",
			"chat_id", chatID, "workspace_id", workspaceID, "err", err)
	}
	if err := u.worktree.DiscardChildWorkspace(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "agent: create chat: discard the workspace nothing came to own",
			"chat_id", chatID, "workspace_id", workspaceID, "err", err)
	}
	return cause
}
