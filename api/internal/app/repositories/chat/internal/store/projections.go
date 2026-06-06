package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc receives every projected Chat for hub fan-out (01 §5).
type BroadcastFunc func(
	c domain.Chat,
)

func registerProjections(
	st storage,
	ax asynx.Asynx[domain.Chat],
	broadcast BroadcastFunc,
) error {
	p := &projector{store: st, broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("chat.*"), p.onEvent); err != nil {
		return fmt.Errorf("chat projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("chat projection: on forget: %w", err)
	}
	return nil
}

type projector struct {
	store     storage
	broadcast BroadcastFunc
}

func (p *projector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	if err := p.saveWithRetry(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "chat projection: save", "id", evt.Aggregate.ID, "err", err)
		return
	}
	p.broadcast(evt.Aggregate)
}

func (p *projector) saveWithRetry(
	ctx context.Context,
	c domain.Chat,
) error {
	var err error
	for range 3 {
		if err = p.store.Save(ctx, c); err == nil {
			return nil
		}
		if !isTransientIOError(err) {
			return err
		}
	}
	return err
}

func isTransientIOError(
	err error,
) bool {
	return strings.Contains(err.Error(), "disk I/O error")
}

func (p *projector) onForget(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	if err := p.store.Delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "chat projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
