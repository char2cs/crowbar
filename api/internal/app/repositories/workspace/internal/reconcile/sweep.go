package reconcile

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SweepListFunc reads the workspace read model directly, never through the
// lazy-Replay List, so boot stays cheap; a lost read model just reaps nothing
// until the first on-demand read rebuilds it.
type SweepListFunc func(ctx context.Context) ([]domain.Workspace, error)

// SweepPurgeFunc re-drives the delete reactor's own idempotent teardown for one
// lingering tombstone, purging from the row's WorktreePath. It is injected so this
// package holds no git/fs/asynx dependency.
type SweepPurgeFunc func(ctx context.Context, tomb domain.Workspace) error

// Sweeper is the boot orphan sweep: it reaps workspaces a crashed delete reactor
// left in Status="deleted", re-driving the same idempotent purge so any interrupted
// step converges to no row and no worktree. Its read and purge are injected.
type Sweeper struct {
	list  SweepListFunc
	purge SweepPurgeFunc
}

// NewSweeper builds the boot orphan sweep over the direct read-model list and the
// idempotent purge.
func NewSweeper(
	list SweepListFunc,
	purge SweepPurgeFunc,
) *Sweeper {
	return &Sweeper{list: list, purge: purge}
}

// Sweep re-drives the purge for every Status="deleted" row, best-effort so recovery
// never fails boot: failures are logged and skipped. onPurged hears each tombstone
// it finished, so a reported give-up can clear.
func (s *Sweeper) Sweep(
	ctx context.Context,
	onPurged func(ctx context.Context, wsID string),
) {
	rows, err := s.list(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "workspace boot orphan-sweep: read model list", "err", err)
		return
	}
	for _, ws := range rows {
		// Cancelable: a shutdown mid-boot (ctx cancel) aborts the remaining reap
		// cleanly rather than logging a purge error per surviving row.
		if ctx.Err() != nil {
			return
		}
		if ws.Status != domain.WorkspaceStatusDeleted {
			continue
		}
		if err := s.purge(ctx, ws); err != nil {
			slog.ErrorContext(ctx, "workspace boot orphan-sweep: purge", "id", ws.ID, "err", err)
			continue
		}
		onPurged(ctx, ws.ID)
	}
}
