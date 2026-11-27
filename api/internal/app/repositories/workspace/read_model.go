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

type ReadModel struct {
	st *projections.Store
}

func OpenReadModel(
	storeDB *gormdb.DB,
) (*ReadModel, error) {
	st, err := projections.NewStore(storeDB)
	if err != nil {
		return nil, fmt.Errorf("workspace: read model: %w", err)
	}
	return &ReadModel{st: st}, nil
}

func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.Workspace]) {
	return projections.Subscriber(rm.st)
}

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
