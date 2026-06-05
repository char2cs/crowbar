package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the AgentRun read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.AgentRun, error)
	ListRunning(
		ctx context.Context,
	) ([]domain.AgentRun, error)
	ListByChat(
		ctx context.Context,
		chatID string,
	) ([]domain.AgentRun, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.AgentRun, error)
}

type storeService struct {
	storage storage
}

// New builds the AgentRun read-model store and registers its projection.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.AgentRun],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("agentrun store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("agentrun store: projections: %w", err)
	}
	return &storeService{storage: st}, nil
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.AgentRun, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) ListRunning(
	ctx context.Context,
) ([]domain.AgentRun, error) {
	all, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return filterByStatus(all, domain.AgentRunStatusRunning), nil
}

func (s *storeService) ListByChat(
	ctx context.Context,
	chatID string,
) ([]domain.AgentRun, error) {
	all, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return filterByChat(all, chatID), nil
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.AgentRun, error) {
	return s.storage.FindByKey(ctx, id)
}

func filterByStatus(
	all []domain.AgentRun,
	status domain.AgentRunStatus,
) []domain.AgentRun {
	result := make([]domain.AgentRun, 0, len(all))
	for _, run := range all {
		if run.Status == status {
			result = append(result, run)
		}
	}
	return result
}

func filterByChat(
	all []domain.AgentRun,
	chatID string,
) []domain.AgentRun {
	result := make([]domain.AgentRun, 0, len(all))
	for _, run := range all {
		if run.ChatID == chatID {
			result = append(result, run)
		}
	}
	return result
}
