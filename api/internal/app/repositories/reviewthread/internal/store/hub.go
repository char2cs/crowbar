package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc receives every projected ReviewThread frame for hub fan-out
// (spec §3.5 delivery). The reviewthread frame is the bare evt.Aggregate — unlike
// the workspace hub it carries no derived overlays — so no enrichment callback is
// needed.
type BroadcastFunc func(
	th domain.ReviewThread,
)

// registerHubProjection subscribes the hub (WS fan-out) projection to every
// review_thread event on the singleton axReviewThread: for each event it hands the
// bare evt.Aggregate to broadcast for hub fan-out. Unlike the retired combined
// projector it does NOT touch the durable read model — the store projection
// (store.go) owns that — so the two derive independently from evt.Aggregate and
// cannot drift (decision 5). Designed to register ONCE on the singleton.
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
// projection owns durability (decision 5).
func (p *hubProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) {
	_ = ctx
	p.broadcast(evt.Aggregate)
}
