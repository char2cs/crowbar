package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// RequestMove records a move for chatID's turn boundary.
func (u *Usecase) RequestMove(ctx context.Context, chatID string, m ledger.Move) (bool, error) {
	return u.runners.RequestMove(ctx, chatID, m)
}

// CancelMove withdraws chatID's pending move.
func (u *Usecase) CancelMove(ctx context.Context, chatID, reason string) error {
	return u.runners.CancelMove(ctx, chatID, reason)
}

// ExecuteMove runs chatID's ready move; see the runner's.
func (u *Usecase) ExecuteMove(
	ctx context.Context, chatID string, apply func(context.Context, ledger.Move) (ledger.Move, error),
) (ledger.Move, bool, error) {
	return u.runners.ExecuteMove(ctx, chatID, apply)
}

// MovesWaiting are the chats whose move a previous daemon left unfinished.
func (u *Usecase) MovesWaiting(ctx context.Context) ([]string, error) {
	return u.runners.MovesWaiting(ctx)
}

// RunIn points chatID at a workspace another row owns.
func (u *Usecase) RunIn(ctx context.Context, chatID, workspaceID string) error {
	if _, err := u.chats.RunIn(ctx, chatID, workspaceID); err != nil {
		return fmt.Errorf("agent: run in %s: %w", workspaceID, err)
	}
	return nil
}

// MoveSite is workspaceID's resolved folder and the project and repo it is in.
func (u *Usecase) MoveSite(ctx context.Context, workspaceID string) (dir, projectID, repoID string, _ error) {
	return u.runners.MoveSite(ctx, workspaceID)
}

// BranchOutWorkspace cuts a branch from workspaceID's own HEAD for a thread made a branch.
func (u *Usecase) BranchOutWorkspace(ctx context.Context, workspaceID, branch string) (domain.Workspace, error) {
	ws, err := u.worktree.CreateBranchOutWorkspace(ctx, workspaceID, branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("agent: branch out: %w", err)
	}
	return ws, nil
}

// CheckBranchOutName refuses a branch name BranchOutWorkspace could not cut from workspaceID.
func (u *Usecase) CheckBranchOutName(ctx context.Context, workspaceID, branch string) error {
	if err := u.worktree.CheckBranchOutName(ctx, workspaceID, branch); err != nil {
		return fmt.Errorf("agent: branch out: %w", err)
	}
	return nil
}

// DiscardWorkspace removes a workspace a failed branch-out cut and nothing owns.
func (u *Usecase) DiscardWorkspace(ctx context.Context, workspaceID string) error {
	return u.worktree.DiscardChildWorkspace(ctx, workspaceID)
}

// RequestBranchOut records that chatID becomes a branch cut from workspaceID at its
// turn boundary; an agent's repeat while one is pending is the same request.
func (u *Usecase) RequestBranchOut(ctx context.Context, chatID, workspaceID, branch, by string) (bool, error) {
	return u.runners.RequestBranchOut(ctx, chatID, workspaceID, branch, by)
}
