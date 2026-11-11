package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc receives every projected ReviewThread for hub fan-out. The frame
// is the bare aggregate: unlike a workspace it carries no derived overlays.
type BroadcastFunc func(
	th domain.ReviewThread,
)

// registerHubProjection hands every review_thread event's aggregate to broadcast.
// It never touches the read model (store.go owns that), so the two derive
// independently and cannot drift. Register it once.
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

// onEvent broadcasts the bare aggregate frame. It never persists — the store
// projection owns durability.
func (p *hubProjector) onEvent(
	_ context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) {
	p.broadcast(evt.Aggregate)
}
