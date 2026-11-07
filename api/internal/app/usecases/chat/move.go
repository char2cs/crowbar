package chat

import (
	"context"
	"fmt"

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
