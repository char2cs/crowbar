package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
)

func sweepCallback(
	ctx context.Context,
	ucs *usecases.Container,
) func(
	wsID string,
	state provider.ProviderState,
) {
	return func(
		wsID string,
		state provider.ProviderState,
	) {
		if err := ucs.ProviderSync.SyncFromState(context.WithoutCancel(ctx), wsID, state, time.Now()); err != nil {
			slog.WarnContext(ctx, "app: provider sweep: apply the polled state", "ws", wsID, "err", err)
		}
	}
}

// shouldSweep reports whether the global cron should re-poll a workspace: it
// must have a live PR (PRUrl != "") and must not be in a terminal PR state
// (pr-merged/pr-closed), which are never re-polled (D10/§11). This widens the
// old Status==pr-open filter so pr-open->pr-merged/closed transitions are
// observed on unwatched workspaces and pr-conflicts workspaces keep syncing.
func shouldSweep(
	ws domain.Workspace,
) bool {
	if ws.PRUrl == "" {
		return false
	}
	if ws.Status == domain.WorkspaceStatusPRMerged ||
		ws.Status == domain.WorkspaceStatusPRClosed {
		return false
	}
	return true
}

func sweepTargets(
	repo workspace.Workspace,
) func() []poll.SweepTarget {
	return func() []poll.SweepTarget {
		rows, err := repo.List(context.Background())
		if err != nil {
			return nil
		}
		targets := make([]poll.SweepTarget, 0, len(rows))
		for _, ws := range rows {
			if !shouldSweep(ws) {
				continue
			}
			targets = append(targets, poll.SweepTarget{
				WSID:      ws.ID,
				RepoPath:  ws.WorktreePath,
				Branch:    ws.Branch,
				HasOpenPR: true,
			})
		}
		return targets
	}
}
