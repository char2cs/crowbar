package reconcile

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SweepListFunc reads the durable workspace read model DIRECTLY — never through
// the lazy-Replay-wrapped List (§3.7). Reading directly keeps boot cheap and, per
// the §3.8 accepted consequence, lets a crash that also lost store/workspace.db
// leave the sweep reading an empty model (it reaps nothing) rather than paying a
// replay to rebuild it; that repair is deferred to the first on-demand access. In
// production this is the workspace repository's raw read-model List.
type SweepListFunc func(ctx context.Context) ([]domain.Workspace, error)

// SweepPurgeFunc re-drives the idempotent teardown for one lingering workspace —
// the SAME purge a delete reactor would run (cascade Forget + rm -rf worktree +
// axWorkspace.Forget, spec §3.6/§3.8). It is injected so this package holds no
// git/fs/asynx dependency, mirroring the reconcile-on-open Reconciler. It must be
// idempotent: a re-drive over an already-purged workspace is a no-op.
type SweepPurgeFunc func(ctx context.Context, wsID string) error

// Sweeper is the cheap, proactive boot orphan-sweep (spec §3.8). A delete reactor
// that crashed mid-cascade leaves a workspace stuck in Status="deleted" with its
// worktree still on disk and its tombstone row still present; the Sweeper reaps
// exactly those, converging to the delete invariant (no row, no worktree). It
// reads the read model directly (no lazy Replay) and re-drives the same
// idempotent purge the reactor would have, so whichever teardown step the crash
// interrupted, the end state is the same. It holds no IO of its own — both the
// direct read and the purge are injected.
type Sweeper struct {
	list  SweepListFunc
	purge SweepPurgeFunc
}

// NewSweeper builds the boot orphan-sweep over the direct read-model list and the
// idempotent purge. Both are supplied by the wiring layer (which owns the git/fs/
// asynx concretes), keeping this package dependency-free.
func NewSweeper(
	list SweepListFunc,
	purge SweepPurgeFunc,
) *Sweeper {
	return &Sweeper{list: list, purge: purge}
}

// Sweep reads the read model directly and re-drives the purge for every residual
// Status="deleted" row. Live (non-deleted) rows are left untouched. It is
// best-effort throughout so boot is never failed by recovery work: a List failure
// is logged and aborts the sweep (there is nothing to iterate), and a per-id
// purge failure is logged and skipped so one stuck row never blocks the rest. It
// triggers NO lazy Replay — an empty model simply yields nothing to reap (§3.8).
func (s *Sweeper) Sweep(
	ctx context.Context,
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
		if err := s.purge(ctx, ws.ID); err != nil {
			slog.ErrorContext(ctx, "workspace boot orphan-sweep: purge", "id", ws.ID, "err", err)
		}
	}
}
