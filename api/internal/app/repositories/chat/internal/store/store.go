package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the chat read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.Chat, error)
}

type storeService struct {
	storage storage
}

// New builds the chat read-model store and registers its projection.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.Chat],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("chat store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("chat store: projections: %w", err)
	}
	return &storeService{storage: st}, nil
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.Chat, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	all, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return filterByWorkspace(all, wsID), nil
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.Chat, error) {
	return s.storage.FindByKey(ctx, id)
}

func filterByWorkspace(
	all []domain.Chat,
	wsID string,
) []domain.Chat {
	result := make([]domain.Chat, 0, len(all))
	for _, c := range all {
		if c.WsID == wsID && c.DeletedAt == nil {
			result = append(result, c)
		}
	}
	return result
}
