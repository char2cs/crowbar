package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const eventKeyPrefix = "events:"

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

type service struct {
	storage storage
	es      asynxModels.Store
	ax      asynx.Asynx[domain.ReviewThread]
}

type Projection struct {
	storage storage
}

func Open(
	db *gormdb.DB,
) (*Projection, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("reviewthread store: %w", err)
	}
	return &Projection{storage: st}, nil
}

func New(
	p *Projection,
	es asynxModels.Store,
	ax asynx.Asynx[domain.ReviewThread],
	broadcast BroadcastFunc,
) (Store, error) {
	if err := registerHubProjection(ax, broadcast); err != nil {
		return nil, fmt.Errorf("reviewthread store: projections: %w", err)
	}
	return &service{storage: p.storage, es: es, ax: ax}, nil
}

func (s *service) List(
	ctx context.Context,
) ([]domain.ReviewThread, error) {
	rows, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	return s.storage.FindAll(ctx)
}

func (s *service) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.ReviewThread, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return filterByWorkspace(all, wsID), nil
}

func (s *service) Get(
	ctx context.Context,
	id string,
) (*domain.ReviewThread, error) {
	return s.storage.FindByKey(ctx, id)
}

func (s *service) rebuild(
	ctx context.Context,
) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("reviewthread store: enumerate aggregate ids: %w", err)
	}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, eventKeyPrefix)
		if !ok {
			continue
		}
		if err := errors.Join(s.replay(ctx, id)); err != nil {
			return fmt.Errorf("reviewthread store: replay %s: %w", id, err)
		}
	}
	return nil
}

func (s *service) replay(
	ctx context.Context,
	id string,
) (replayErr, saveErr error) {
	replayErr = s.ax.Replay(ctx, id, 1, 0, func(ctx context.Context, evt asynxModels.Event[domain.ReviewThread]) {
		switch {
		case saveErr != nil:
		case evt.EventName == asynxModels.ForgetEventName:
			saveErr = s.storage.Delete(ctx, id)
		default:
			saveErr = s.storage.Save(ctx, evt.Aggregate)
		}
	})
	return replayErr, saveErr
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

func Subscriber(
	p *Projection,
) func(*asynx.Builder[domain.ReviewThread]) {
	return func(b *asynx.Builder[domain.ReviewThread]) {
		b.WithSubscriber(asynx.Topic("review_thread.*"), p.handle)
	}
}

func (p *Projection) handle(
	ctx context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) error {
	if evt.EventName == asynxModels.ForgetEventName {
		if err := p.storage.Delete(ctx, evt.AggregateID); err != nil {
			return fmt.Errorf("reviewthread store projection: delete %q: %w", evt.AggregateID, err)
		}
		return nil
	}
	if err := p.storage.Save(ctx, evt.Aggregate); err != nil {
		return fmt.Errorf("reviewthread store projection: save %q: %w", evt.Aggregate.ID, err)
	}
	return nil
}
