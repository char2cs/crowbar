package projections

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RegisterHub subscribes the hub (WS fan-out) projection to every workspace event
// on the singleton axWorkspace. For each event it takes the base aggregate
// (evt.Aggregate), runs the injected enrich callback to attach the derived
// overlays that are NOT part of the event-sourced state — the Working/inflight
// spinner and the merge-eligibility overlay (CanMergeLocally/ParentBranch) — then
// hands the resulting frame to broadcast for hub fan-out (spec §3.5 hub-frame
// enrichment, decision 5).
//
// enrich and broadcast are owned by repositories.Container: the SAME pair also
// backs the BeginWork/EndWork request-bracketed rebroadcasts (which fire on the
// 202 ack, not on an event), so the emitted frame is identical regardless of
// whether it originated from a committed event or a spinner transition. The frame
// type F is a parameter so this package stays decoupled from the api-layer wire
// DTO the container supplies.
//
// Unlike the retired combined projector it does NOT touch the durable read model
// — the store projection (store.go) owns that. The two derive independently from
// evt.Aggregate and cannot drift (decision 5). Designed to register ONCE on the
// singleton, not per aggregate.
func RegisterHub[F any](
	ax asynx.Asynx[domain.Workspace],
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	p := &hubProjector[F]{enrich: enrich, broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("workspace.*"), p.onEvent); err != nil {
		return fmt.Errorf("workspace hub projection: subscribe: %w", err)
	}
	return nil
}

type hubProjector[F any] struct {
	enrich    func(ctx context.Context, ws domain.Workspace) F
	broadcast func(frame F)
}

// onEvent derives the base frame from evt.Aggregate, enriches it with the
// request-scoped/derived overlays, and broadcasts it. It never persists — the
// store projection owns durability (decision 5).
func (p *hubProjector[F]) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	p.broadcast(p.enrich(ctx, evt.Aggregate))
}
