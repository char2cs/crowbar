package projections

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RegisterHub broadcasts every workspace event, enriched with the derived overlays, from the store
// projection after its save: a separate subscriber could beat the read model and serve stale state.
// enrich/broadcast are shared with turn rebroadcasts; tombstone frames are reported via Announced.
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
