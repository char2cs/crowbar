package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// SpawnChatWithImportedWorktree fills chatID's empty workspace slot with a
// branch that ALREADY EXISTS — the import counterpart of
// SpawnChatWithOwnWorktree (own_worktree.go), through the same WorktreeCreator
// port — and starts providerID's CLI in it.
//
// The two differ in exactly one place, and it is the fork parent. The fork path
// has to RESOLVE one, by walking chatID's own just-written placement, because
// there is nothing else to say where a brand-new branch comes from. An import
// already knows: the caller discovered this branch in a repository and resolved
// its PR-base lineage before asking, so spec names the repo and the parent
// outright and no walk is taken. Everything after that — set the slot, start
// the CLI, take the workspace back out if either fails — is the identical
// sequence, and deliberately so.
//
// An empty providerID starts no CLI and returns an empty runner id. Repo-add
// and batch import materialise many branches at once and none of them is a
// conversation anybody has opened; starting a vendor CLI per imported branch
// would launch a process per row for rows the user has not asked to talk to.
//
// A failure after the workspace is minted takes it back out, for the reason
// discardMintedWorkspace's own doc gives: the caller
// (createImportedWorktreeChat, chat/internal/tree/imported_chat.go) purges
// chatID outright on any error from here, so a workspace left behind is not
// protected by the row that named it — it is simply unreachable.
func (u *Usecase) SpawnChatWithImportedWorktree(
	ctx context.Context,
	chatID string,
	providerID string,
	spec tree.ImportSpec,
) (domain.Workspace, string, error) {
	ws, err := u.worktree.CreateImportedWorkspace(ctx, spec)
	if err != nil {
		return domain.Workspace{}, "", fmt.Errorf("import chat: create workspace: %w", err)
	}
	if _, err := u.chats.SetWorkspace(ctx, chatID, ws.ID); err != nil {
		return domain.Workspace{}, "", u.discardUnclaimedWorkspace(ctx, ws.ID,
			fmt.Errorf("import chat: set workspace: %w", err))
	}
	if providerID == "" {
		return ws, "", nil
	}
	runnerID, err := u.runners.StartRunner(ctx, chatID, providerID)
	if err != nil {
		return domain.Workspace{}, "", u.discardMintedWorkspace(ctx, chatID, ws.ID,
			fmt.Errorf("import chat: start runner: %w", err))
	}
	return ws, runnerID, nil
}

// AttachWorkspace points an already-minted chat at the workspace it owns. It is
// the bare slot write behind the tree usecase's MintOwningChat sequence
// (chat/internal/tree/owning_chat.go), for the creation paths that build their
// own workspace and only need the row pointed at it.
func (u *Usecase) AttachWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
) error {
	if _, err := u.chats.SetWorkspace(ctx, chatID, workspaceID); err != nil {
		return fmt.Errorf("agent: attach workspace %s to chat %s: %w", workspaceID, chatID, err)
	}
	return nil
}
