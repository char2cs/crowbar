package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the ReviewThread read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.ReviewThread, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.ReviewThread, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.ReviewThread, error)
}

type storeService struct {
	storage storage
}

// New builds the ReviewThread read-model store and registers its projection.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.ReviewThread],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("reviewthread store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("reviewthread store: projections: %w", err)
	}
	return &storeService{storage: st}, nil
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.ReviewThread, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.ReviewThread, error) {
	all, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return filterByWorkspace(all, wsID), nil
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.ReviewThread, error) {
	return s.storage.FindByKey(ctx, id)
}

func filterByWorkspace(
	all []domain.ReviewThread,
	wsID string,
) []domain.ReviewThread {
	result := make([]domain.ReviewThread, 0, len(all))
	for _, th := range all {
		if th.WsID == wsID {
			result = append(result, th)
		}
	}
	return result
}
