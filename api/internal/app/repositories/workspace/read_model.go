package workspace

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReadModel is the durable workspace read model (state/store/workspace.db). It
// is opened before axWorkspace so it can be that instance's durable subscriber.
type ReadModel struct {
	st *projections.Store
}

// OpenReadModel opens the read model over storeDB.
func OpenReadModel(
	storeDB *gormdb.DB,
) (*ReadModel, error) {
	st, err := projections.NewStore(storeDB)
	if err != nil {
		return nil, fmt.Errorf("workspace: read model: %w", err)
	}
	return &ReadModel{st: st}, nil
}

// Subscriber is the builder option that makes rm axWorkspace's durable subscriber.
func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.Workspace]) {
	return projections.Subscriber(rm.st)
}

// RegisterHubProjection registers the hub (WS fan-out) projection on repo's
// axWorkspace: each event's frame is built from evt.Aggregate, enriched with the
// container's overlays, broadcast, and its tombstone reported to the read model.
func RegisterHubProjection[F any](
	repo Workspace,
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	w, ok := repo.(*workspace)
	if !ok {
		return fmt.Errorf("workspace: hub projection needs the concrete repository")
	}
	return store.RegisterHub(w.readModel, enrich, broadcast)
}
