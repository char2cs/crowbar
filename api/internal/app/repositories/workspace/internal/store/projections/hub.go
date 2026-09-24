package projections

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RegisterHub makes st announce every workspace event on the hub (WS fan-out)
// once the event is durable in the read model. For each event it takes the
// base aggregate, runs the injected enrich callback to attach the derived
// overlays that are NOT part of the event-sourced state — the Working/inflight
// spinner and the merge-eligibility overlay (CanMergeLocally/ParentBranch) —
// then hands the frame to broadcast (spec §3.5 hub-frame enrichment).
//
// The frame is sent by the store projection itself, after its save, rather than
// by a second subscriber: asynx runs an event's subscribers concurrently, so a
// separate hub subscriber could put a frame on the wire before the read model
// held it, and a client that re-read the model on that frame got the older
// state back.
//
// enrich and broadcast are owned by repositories.Container: the SAME pair also
// backs the BeginWork/EndWork request-bracketed rebroadcasts, so the emitted
// frame is identical whichever path fired. The frame type F is a parameter so
// this package stays decoupled from the api-layer wire DTO.
//
// Every tombstone frame is reported to st (Store.Announced): the delete reactor
// purges a tombstone — the workspace's owning chat included — only after its
// frame has gone out.
func RegisterHub[F any](
	st *Store,
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) {
	announce := func(ctx context.Context, ws domain.Workspace) {
		broadcast(enrich(ctx, ws))
		st.Announced(ws)
	}
	st.announce.Store(&announce)
	st.ExpectAnnouncements()
}
