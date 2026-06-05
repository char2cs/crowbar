package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the workspace read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
}

type storeService struct {
	storage storage
}

// New builds the read-model store, registering the projection that keeps it in
// sync with the aggregate and fans every row out through broadcast.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.Workspace],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("workspace store: projections: %w", err)
	}
	return &storeService{storage: st}, nil
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	return s.storage.FindByKey(ctx, id)
}
