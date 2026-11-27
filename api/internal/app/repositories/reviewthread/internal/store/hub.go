package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type BroadcastFunc func(
	th domain.ReviewThread,
)

func registerHubProjection(
	ax asynx.Asynx[domain.ReviewThread],
	broadcast BroadcastFunc,
) error {
	p := &hubProjector{broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("review_thread.*"), p.onEvent); err != nil {
		return fmt.Errorf("reviewthread hub projection: subscribe: %w", err)
	}
	return nil
}

type hubProjector struct {
	broadcast BroadcastFunc
}

func (p *hubProjector) onEvent(
	_ context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) {
	p.broadcast(evt.Aggregate)
}
