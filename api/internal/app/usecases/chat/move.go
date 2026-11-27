package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func (u *Usecase) RequestMove(ctx context.Context, chatID string, m ledger.Move) (bool, error) {
	return u.runners.RequestMove(ctx, chatID, m)
}

func (u *Usecase) CancelMove(ctx context.Context, chatID, reason string) error {
	return u.runners.CancelMove(ctx, chatID, reason)
}

func (u *Usecase) ExecuteMove(
	ctx context.Context, chatID string, apply func(context.Context, ledger.Move) (ledger.Move, error),
) (ledger.Move, bool, error) {
	return u.runners.ExecuteMove(ctx, chatID, apply)
}

func (u *Usecase) MovesWaiting(ctx context.Context) ([]string, error) {
	return u.runners.MovesWaiting(ctx)
}

func (u *Usecase) RunIn(ctx context.Context, chatID, workspaceID string) error {
	if _, err := u.chats.RunIn(ctx, chatID, workspaceID); err != nil {
		return fmt.Errorf("agent: run in %s: %w", workspaceID, err)
	}
	return nil
}

func (u *Usecase) MoveSite(ctx context.Context, workspaceID string) (dir, projectID, repoID string, _ error) {
	return u.runners.MoveSite(ctx, workspaceID)
}

func (u *Usecase) BranchOutWorkspace(ctx context.Context, workspaceID, branch string) (domain.Workspace, error) {
	ws, err := u.worktree.CreateBranchOutWorkspace(ctx, workspaceID, branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("agent: branch out: %w", err)
	}
	return ws, nil
}

func (u *Usecase) CheckBranchOutName(ctx context.Context, workspaceID, branch string) error {
	if err := u.worktree.CheckBranchOutName(ctx, workspaceID, branch); err != nil {
		return fmt.Errorf("agent: branch out: %w", err)
	}
	return nil
}

func (u *Usecase) DiscardWorkspace(ctx context.Context, workspaceID string) error {
	return u.worktree.DiscardChildWorkspace(ctx, workspaceID)
}

func (u *Usecase) RequestBranchOut(ctx context.Context, chatID, workspaceID, branch, by string) (bool, error) {
	return u.runners.RequestBranchOut(ctx, chatID, workspaceID, branch, by)
}
