package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
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

// New builds the ReviewThread read-model store, registers the projection that
// keeps it in sync with the aggregate, then reconciles every read-model row from
// the authoritative event store for each id in aggregateIDs.
//
// The event log and the read model are separate SQLite DBs written without a
// shared transaction, and the projection runs asynchronously after the event is
// durable; a crash (or a dropped projection) in that window leaves the event
// durable but the read-model row stale or missing. Because List reads the read
// model, without reconcile-on-open a thread would be missing or stale forever
// after a mid-projection crash, while Get (which reads the event store) stays
// correct — a permanent divergence. The reconcile is best-effort: a failed row
// never blocks opening the store, and the next write self-corrects it. The
// reviewthread aggregate is global (one store + one asynx for every thread), so
// the repo enumerates the event store's aggregate ids and passes them here.
func New(
	ctx context.Context,
	db *gormdb.DB,
	ax asynx.Asynx[domain.ReviewThread],
	broadcast BroadcastFunc,
	aggregateIDs []string,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("reviewthread store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("reviewthread store: projections: %w", err)
	}
	svc := &storeService{storage: st}
	svc.reconcile(ctx, ax, aggregateIDs)
	return svc, nil
}

// reconcile overwrites each read-model row in aggregateIDs with the current state
// replayed from the event store. An id with no events is a no-op. Each id is
// best-effort: a failure is logged and skipped so one bad row never blocks the
// rest or the open.
func (s *storeService) reconcile(
	ctx context.Context,
	ax asynx.Asynx[domain.ReviewThread],
	aggregateIDs []string,
) {
	for _, id := range aggregateIDs {
		if err := s.reconcileOne(ctx, ax, id); err != nil {
			slog.WarnContext(ctx, "reviewthread store: read-model reconcile failed; self-corrects on next write",
				"aggregate", id, "err", err)
		}
	}
}

func (s *storeService) reconcileOne(
	ctx context.Context,
	ax asynx.Asynx[domain.ReviewThread],
	id string,
) error {
	th, err := ax.Get(ctx, id)
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil
		}
		return err
	}
	if th.ID == "" {
		return nil
	}
	return s.storage.Save(ctx, th)
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
