package projections

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
