// Package store owns the reviewthread read model: the durable, queryable
// projection of the aggregate at state/store/review_thread.db. New builds the read
// model over the read-pool DB and registers TWO distinct projections on the
// singleton axReviewThread (spec §3.5/§3.7, decision 5): the SAVE-ONLY store
// projection here (store.go) folds evt.Aggregate into the durable read model and,
// on Forget, deletes the row; the hub projection (hub.go) owns WS fan-out. The two
// derive independently from evt.Aggregate and cannot drift.
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

// New builds the durable read model over db (state/store/review_thread.db) and
// registers both read-side projections on ax, once, for the singleton
// axReviewThread: the save-only store projection (this file) and the hub broadcast
// projection (hub.go). It performs no reconcile — read-model repair is lazy. es is
// the same per-type event log ax wraps, retained so List can reach its
// serialize.AggregateLister capability for the whole-model rebuild (§3.7).
func New(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.ReviewThread],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("reviewthread store: %w", err)
	}
	if err := registerStoreProjection(st, ax); err != nil {
		return nil, fmt.Errorf("reviewthread store: projections: %w", err)
	}
	if err := registerHubProjection(ax, broadcast); err != nil {
		return nil, fmt.Errorf("reviewthread store: projections: %w", err)
	}
	return &service{storage: st, es: es, ax: ax}, nil
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

// registerStoreProjection subscribes the SAVE-ONLY read-model projection to every
// review_thread event on the singleton axReviewThread: it folds evt.Aggregate into
// the durable read model and, on Forget, deletes the aggregate's row (a cheap,
// synchronous OnForget row-delete — spec §3.6, no fs/git/network io). Unlike the
// retired combined projector it does NOT broadcast; the hub projection (hub.go)
// owns fan-out (decision 5). Designed to register ONCE on the singleton.
func registerStoreProjection(
	st storage,
	ax asynx.Asynx[domain.ReviewThread],
) error {
	p := &storeProjector{storage: st}
	if _, err := ax.Subscribe(asynx.Topic("review_thread.*"), p.onEvent); err != nil {
		return fmt.Errorf("reviewthread store projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("reviewthread store projection: on forget: %w", err)
	}
	return nil
}

type storeProjector struct {
	storage storage
}

func (p *storeProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) {
	if err := p.saveWithRetry(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "reviewthread store projection: save", "id", evt.Aggregate.ID, "err", err)
	}
}

func (p *storeProjector) saveWithRetry(
	ctx context.Context,
	th domain.ReviewThread,
) error {
	var err error
	for range 3 {
		if err = p.storage.Save(ctx, th); err == nil {
			return nil
		}
		if !isTransientIOError(err) {
			return err
		}
	}
	return err
}

func isTransientIOError(
	err error,
) bool {
	return strings.Contains(err.Error(), "disk I/O error")
}

func (p *storeProjector) onForget(
	ctx context.Context,
	evt asynxModels.Event[domain.ReviewThread],
) {
	if err := p.storage.Delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "reviewthread store projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
