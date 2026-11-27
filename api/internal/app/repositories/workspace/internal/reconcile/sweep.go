package reconcile

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type SweepListFunc func(ctx context.Context) ([]domain.Workspace, error)

type SweepPurgeFunc func(ctx context.Context, tomb domain.Workspace) error

type Sweeper struct {
	list  SweepListFunc
	purge SweepPurgeFunc
}

func NewSweeper(
	list SweepListFunc,
	purge SweepPurgeFunc,
) *Sweeper {
	return &Sweeper{list: list, purge: purge}
}

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
