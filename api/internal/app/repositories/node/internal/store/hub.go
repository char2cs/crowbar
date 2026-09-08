package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventNamePrefix is the aggregate-name segment every node command's
// EventName() puts first ("node.<kind>.<id>" — see internal/commands/*.go).
// eventKind strips it to isolate <kind>.
const eventNamePrefix = "node."

// NodeEvent is one projected node lifecycle event, in repo-owned terms: the
// bare (nodeID, parentID, order, kind) facts a live position update is built
// from. This is the wire shape a future sidebar hub broadcast wires straight
// through with no adapter, mirroring agentchat's ChatEvent — the frontend needs
// live position updates for cross-tab/cross-client drag sync, the same as chat
// rows already get.
//
// kind is the <kind> segment of the emitting command's EventName
// ("node.<kind>.<id>"), so the lifecycle vocabulary is exactly one term per
// command:
//
//	created         — a fresh Node minted (new chat, folder, repo import, workspace fork/lock)
//	order_set       — a densify pass renumbered this row within its existing parent
//	placement_set   — this row moved to a new parent and/or index (the one row actually dragged)
//	deleted         — the node was hard-deleted (asynx Forget, see OnForget below;
//	                  NOT emitted via a command's EventName like the other kinds)
type NodeEvent struct {
	NodeID    string
	ParentID  string
	Order     int
	Kind      string
	Forgotten bool
}

// WatchFunc receives every projected node event. The repository announces WHAT
// HAPPENED; a future usecase-layer fanout decides what the frontend is told —
// mirrors agentchat's WatchFunc. Registering the subscription is the
// repository's job only because asynx subscription is; shaping a wire frame is
// not.
type WatchFunc func(NodeEvent)

// registerHubProjection subscribes the hub (WS fan-out) projection to every node
// event on the singleton axNode: for each event it derives the lifecycle kind
// from evt.EventName ("node.<kind>.<id>", set by internal/commands/*.go) and
// hands (evt.AggregateID, kind, parentID, order) to watch. It does NOT touch the
// durable read model, so it and the store projection (store.go) derive
// independently from the same event stream and cannot drift. Designed to
// register ONCE on the singleton.
func registerHubProjection(
	ax asynx.Asynx[domain.Node],
	watch WatchFunc,
) error {
	p := &hubProjector{watch: watch}
	if _, err := ax.Subscribe(asynx.Topic("node.*"), p.onEvent); err != nil {
		return fmt.Errorf("node hub projection: subscribe: %w", err)
	}
	// ax.Forget (the hard-delete path) fires ONLY the "asynx.aggregate.forget"
	// topic via OnForget — it is not one of the commands Subscribe's "node.*"
	// pattern matches — so a Forget would otherwise never reach the hub.
	// evt.Aggregate carries the aggregate's last-known state at the moment it
	// was forgotten, exactly like every other projected event.
	if _, err := ax.OnForget(func(_ context.Context, evt asynxModels.Event[domain.Node]) {
		p.onForget(evt)
	}); err != nil {
		return fmt.Errorf("node hub projection: onforget: %w", err)
	}
	return nil
}

type hubProjector struct {
	watch WatchFunc
}

// emit is the single exit. A nil watch degrades to a no-op so a store built
// without one (every unit test, and app.New until a live-update consumer wires
// one) never panics — mirrors agentchat's hub projection.
func (p *hubProjector) emit(e NodeEvent) {
	if p.watch == nil {
		return
	}
	p.watch(e)
}

// onForget announces a hard delete. A forgotten node is not anywhere in the
// tree any more; the client drops it.
func (p *hubProjector) onForget(evt asynxModels.Event[domain.Node]) {
	p.emit(NodeEvent{
		NodeID:    evt.Aggregate.ID,
		ParentID:  evt.Aggregate.ParentID,
		Order:     evt.Aggregate.Order,
		Kind:      "deleted",
		Forgotten: true,
	})
}

// onEvent broadcasts the (nodeID, parentID, order, kind) lifecycle frame
// derived from the event. parentID/order come off the REDUCED aggregate
// (evt.Aggregate), not the event name/id — neither is encoded in EventName. It
// never persists — the store projection owns durability.
func (p *hubProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Node],
) {
	_ = ctx
	p.emit(NodeEvent{
		NodeID:   evt.AggregateID,
		ParentID: evt.Aggregate.ParentID,
		Order:    evt.Aggregate.Order,
		Kind:     eventKind(evt.EventName),
	})
}

// eventKind extracts the <kind> segment from a node EventName
// ("node.<kind>.<id>"). Falls back to the name with only the aggregate prefix
// stripped if a future command ever emits a name that doesn't fit the pattern,
// rather than silently dropping the frame.
func eventKind(
	eventName string,
) string {
	rest := strings.TrimPrefix(eventName, eventNamePrefix)
	kind, _, found := strings.Cut(rest, ".")
	if !found {
		return rest
	}
	return kind
}
