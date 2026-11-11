package tree

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrNoRepoToBranch refuses making a branch of a thread whose folder is no repository.
var ErrNoRepoToBranch = problem.New(problem.Conflict, "A thread in the project home has no repository to branch from")

// BranchOut makes chatID a branch at its turn boundary: a new worktree cut from its
// folder's own HEAD, which the thread then owns and runs in. branch "" is generated.
func (u *chatFolderUsecase) BranchOut(ctx context.Context, chatID, branch string) error {
	chat, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent chat folder: branch out %s: %w", chatID, err)
	}
	if !chat.IsChat() || chat.OwnsWorkspace {
		return ErrOwnerMoves
	}
	repoID, err := u.workspaces.RepoOf(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent chat folder: branch out %s: %w", chatID, err)
	}
	if repoID == "" {
		return ErrNoRepoToBranch
	}
	// The person is told now, while they can still edit the name, not after the turn ends.
	if err := u.agent.CheckBranchOutName(ctx, chat.WorkspaceID, branch); err != nil {
		return fmt.Errorf("agent chat folder: branch out %s: %w", chatID, err)
	}
	ready, err := u.agent.RequestBranchOut(ctx, chatID, chat.WorkspaceID, branch, "user")
	if err != nil {
		return fmt.Errorf("agent chat folder: branch out %s: %w", chatID, err)
	}
	if ready {
		u.MoveWhenReady(ctx, chatID)
	}
	return nil
}

// applyBranchOut cuts the branch and hands chatID its ownership, keeping the row where
// it is; a failure after the worktree exists removes it, so no orphan is left on disk.
func (u *chatFolderUsecase) applyBranchOut(ctx context.Context, chatID string, m ledger.Move) (ledger.Move, error) {
	ws, err := u.agent.BranchOutWorkspace(ctx, m.From, m.Branch)
	if err != nil {
		return ledger.Move{}, err
	}
	if err := u.agent.AttachWorkspace(ctx, chatID, ws.ID); err != nil {
		return ledger.Move{}, errors.Join(err, u.agent.DiscardWorkspace(ctx, ws.ID))
	}
	to, err := u.siteOf(ctx, ws.ID)
	if err != nil {
		to.Dir = ws.WorktreePath
	}
	return ledger.Move{
		From: m.From, To: ws.ID, FromDir: m.FromDir, ToDir: to.Dir, BranchOut: true,
		Branch: ws.Branch, Base: ws.ForkPointSha, By: m.By,
	}, nil
}
