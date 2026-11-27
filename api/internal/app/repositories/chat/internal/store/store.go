package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/core/readonce"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const eventKeyPrefix = "events:"

var ErrNotFound = errors.New("agentchat store: not found")

type Store interface {
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

type service struct {
	storage    storage
	es         asynxModels.Store
	ax         asynx.Asynx[domain.Chat]
	rebuiltFor atomic.Pointer[string]
}

type Projection struct {
	storage storage
}

func Open(
	db *gormdb.DB,
) (*Projection, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("agentchat store: %w", err)
	}
	return &Projection{storage: st}, nil
}

func New(
	p *Projection,
	es asynxModels.Store,
	ax asynx.Asynx[domain.Chat],
	watch WatchFunc,
) (Store, error) {
	if err := registerHubProjection(ax, watch); err != nil {
		return nil, fmt.Errorf("agentchat store: projections: %w", err)
	}
	return &service{storage: p.storage, es: es, ax: ax}, nil
}

func (s *service) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	if id == "" {
		return domain.Chat{}, fmt.Errorf("agentchat store: get %q: %w", id, ErrNotFound)
	}
	chat, err := s.storage.FindByKey(ctx, id)
	if err != nil {
		return domain.Chat{}, err
	}
	if chat == nil {
		if chat, err = s.healOne(ctx, id); err != nil {
			return domain.Chat{}, err
		}
	}
	if chat == nil {
		return domain.Chat{}, fmt.Errorf("agentchat store: get %q: %w", id, ErrNotFound)
	}
	return *chat, nil
}

func (s *service) healOne(ctx context.Context, id string) (*domain.Chat, error) {
	if err := errors.Join(s.replay(ctx, id)); err != nil {
		return nil, fmt.Errorf("agentchat store: heal %q: %w", id, err)
	}
	return s.storage.FindByKey(ctx, id)
}

func (s *service) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	all, err := s.allHealed(ctx)
	if err != nil {
		return nil, err
	}
	return all, nil
}

func (s *service) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	all, err := s.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Chat, 0, len(all))
	for _, chat := range all {
		if chat.WorkspaceID == wsID {
			result = append(result, chat)
		}
	}
	return result, nil
}

func (s *service) allHealed(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := readonce.Do(ctx, "agentchat:all", func() ([]domain.Chat, error) { return s.healedRows(ctx) })
	return slices.Clone(rows), err
}

func Subscriber(
	p *Projection,
) func(*asynx.Builder[domain.Chat]) {
	return func(b *asynx.Builder[domain.Chat]) {
		b.WithSubscriber(asynx.Topic("agentchat.*"), p.handle)
	}
}

func (p *Projection) handle(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) error {
	if evt.EventName == asynxModels.ForgetEventName {
		if err := p.storage.Delete(ctx, evt.AggregateID); err != nil {
			return fmt.Errorf("agentchat store projection: delete %q: %w", evt.AggregateID, err)
		}
		return nil
	}
	if err := p.storage.Save(ctx, evt.Aggregate); err != nil {
		return fmt.Errorf("agentchat store projection: save %q: %w", evt.Aggregate.ID, err)
	}
	return nil
}
