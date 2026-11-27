package tree

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var ErrNoRepoToBranch = problem.New(problem.Conflict, "A thread in the project home has no repository to branch from")

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
