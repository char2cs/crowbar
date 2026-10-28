// Package store owns the reviewthread read model: the durable, queryable
// projection of the aggregate at state/store/review_thread.db. Two projections
// derive from the same stream: the save-only Projection here, axReviewThread's
// durable subscriber, and the live hub projection (hub.go).
//
// Unlike the retired half-aligned store it does NO eager reconcile on open — a
// normal boot re-opens the durable read model with zero replay. Read-model repair
// is lazy: List heals a lost model on first access via whole-model asynx.Replay
// (spec §3.7), enumerating every id the event log holds.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it stores
// that aggregate's events (its snapshots use "snapshots:"). The crowbar event
// store's AggregateLister returns the RAW store keys — both "events:<id>" and
// "snapshots:<id>" — so rebuild must keep only the events keys and strip this
// prefix to recover the real aggregate id, which asynx.Replay re-prepends itself.
const eventKeyPrefix = "events:"

// Store is the ReviewThread read model: a projected, queryable view of the
// aggregate. Per-id aggregate reads fold from the event log via axReviewThread.Get
// in the repo; this surface serves the projected List/Get.
type Store interface {
	// List returns every thread in the durable read model, first healing it via
	// whole-model lazy Replay when the model is empty but the event log still holds
	// aggregates (spec §3.7).
	List(
		ctx context.Context,
	) ([]domain.ReviewThread, error)
	// ListByWorkspace returns the threads for wsID, healing the read model the same
	// way List does before filtering.
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.ReviewThread, error)
	// Get returns the read-model row for id, or nil when absent.
	Get(
		ctx context.Context,
		id string,
	) (*domain.ReviewThread, error)
}

// service is the reviewthread read model over the durable store projection. It
// holds es (the event log's serialize.AggregateLister source) and ax (for Replay)
// so List can rebuild a lost read model on demand.
type service struct {
	storage storage
	es      asynxModels.Store
	ax      asynx.Asynx[domain.ReviewThread]
}

// Projection is the read model's write side. It is opened before
// axReviewThread so it can be that instance's durable subscriber.
type Projection struct {
	storage storage
}

// Open opens the read model over db (state/store/review_thread.db).
func Open(
	db *gormdb.DB,
) (*Projection, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("reviewthread store: %w", err)
	}
	return &Projection{storage: st}, nil
}

// New builds the read model over p, whose instance ax was built with
// Subscriber(p), and registers the hub projection on ax. es is the log ax
// wraps, kept so List can rebuild a lost model (§3.7).
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

// List returns the durable read model, first healing it via a whole-model lazy
// Replay repair when the model is empty but the event log still holds aggregates
// (spec §3.7, decision 7).
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

// ListByWorkspace heals the read model (via List) then filters to wsID.
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

// Get returns the read-model row for id (nil when absent). It never rebuilds — a
// per-id aggregate read folds directly from the event log in the repo.
func (s *service) Get(
	ctx context.Context,
	id string,
) (*domain.ReviewThread, error) {
	return s.storage.FindByKey(ctx, id)
}

// rebuild enumerates every aggregate id in the event log and Replays each into the
// read model. It is a no-op when the event store cannot enumerate its ids or when
// the log is empty (nothing to Replay), so an empty model over an empty log stays
// empty.
//
// asynx.Replay rebuilds a SINGLE aggregate id, so a lost List read model cannot be
// healed by one call: rebuild enumerates every id the event log holds via the
// event store's serialize.AggregateLister capability, then Replays each id back
// into state/store/review_thread.db.
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
			// A non-event key (e.g. a "snapshots:<id>" row for the same aggregate)
			// — skip it; the aggregate is rebuilt from its "events:" key.
			continue
		}
		if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
			return fmt.Errorf("reviewthread store: replay %s: %w", id, err)
		}
	}
	return nil
}

// foldReplayed persists each replayed aggregate into the read model. Replay folds
// the event stream, so evt.Aggregate carries the aggregate's state at that version;
// the final event leaves the current state durable.
func (s *service) foldReplayed(
	ctx context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) {
	if err := s.storage.Save(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "reviewthread store: replay fold", "id", evt.Aggregate.ID, "err", err)
	}
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

// Subscriber registers p as axReviewThread's durable subscriber: a failed save
// is retried by asynx, and parks the aggregate, instead of being logged and
// dropped. The Forget marker deletes the row; a missing row is no error.
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
