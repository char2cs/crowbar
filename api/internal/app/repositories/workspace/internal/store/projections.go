package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc receives every projected Workspace row for hub fan-out (03 §2).
type BroadcastFunc func(
	ws domain.Workspace,
)

// registerProjections subscribes to all workspace events, upserts the read model,
// and broadcasts the complete row. One producer ever emits a Workspace (03 §2).
func registerProjections(
	st storage,
	ax asynx.Asynx[domain.Workspace],
	broadcast BroadcastFunc,
) error {
	p := &projector{store: st, broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("workspace.*"), p.onEvent); err != nil {
		return fmt.Errorf("workspace projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("workspace projection: on forget: %w", err)
	}
	return nil
}

type projector struct {
	store     storage
	broadcast BroadcastFunc
}

func (p *projector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	if err := p.store.Save(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "workspace projection: save", "id", evt.Aggregate.ID, "err", err)
		return
	}
	p.broadcast(evt.Aggregate)
}

func (p *projector) onForget(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	if err := p.store.Delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "workspace projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
