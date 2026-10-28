// Package store owns the node read model: the durable, queryable projection of
// the domain.Node aggregate at state/store/node.db. Two projections derive
// from the same stream: the save-only Projection here, axNode's durable
// subscriber, and the live hub projection (hub.go).
//
// Like agentchat it does NO eager reconcile on open — a normal boot re-opens the
// durable read model with zero replay. Read-model repair is lazy: every read
// heals a lost model on first access via whole-model asynx.Replay, enumerating
// every id the event log holds.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it
// stores that aggregate's events (its snapshots use "snapshots:"). The crowbar
// event store's AggregateLister returns the RAW store keys — both "events:<id>"
// and "snapshots:<id>" — so rebuild must keep only the events keys and strip
// this prefix to recover the real aggregate id, which asynx.Replay re-prepends
// itself.
const eventKeyPrefix = "events:"

// ErrNotFound is returned when no read-model row exists for a requested node,
// even after a whole-model rebuild attempt. Defined locally (rather than
// importing the parent node package's sentinel) because that package imports
// this store package — importing back would cycle. The parent's event_store.go
// bridges this sentinel to node.ErrNotFound (mapNotFound).
var ErrNotFound = errors.New("node store: not found")

// Store is the Node read model: a projected, queryable view of the aggregate.
// Every read heals a lost read model via whole-model lazy Replay (see rebuild)
// before concluding a node genuinely doesn't exist.
type Store interface {
	// GetNode returns the node for id, healing the read model first if it is
	// empty.
	GetNode(
		ctx context.Context,
		id string,
	) (domain.Node, error)
	// ListByParent returns every node whose ParentID is parentID — the
	// sibling-space read every densify pass needs — healing the read model the
	// same way GetNode does before filtering.
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]domain.Node, error)
	// ListAll returns every node in one read, healing the read model the same
	// way — for a caller that would otherwise ask ListByParent per level.
	ListAll(
		ctx context.Context,
	) ([]domain.Node, error)
}

// service is the node read model over the durable store projection. It holds es
// (the event log's serialize.AggregateLister source) and ax (for Replay) so
// reads can rebuild a lost read model on demand.
type service struct {
	storage storage
	es      asynxModels.Store
	ax      asynx.Asynx[domain.Node]
}

// Projection is the read model's write side. It is opened before the asynx
// instance so it can be that instance's durable subscriber (Subscriber).
type Projection struct {
	storage   storage
	placement func(id string)
}

// Open opens the read model over db (state/store/node.db).
func Open(
	db *gormdb.DB,
	opts ...Option,
) (*Projection, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("node store: %w", err)
	}
	p := &Projection{storage: st}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// New builds the read model over p, whose instance ax was built with
// Subscriber(p), and registers the hub projection on ax. es is the log ax
// wraps, kept for the lazy whole-model rebuild.
func New(
	p *Projection,
	es asynxModels.Store,
	ax asynx.Asynx[domain.Node],
	watch WatchFunc,
) (Store, error) {
	if err := registerHubProjection(ax, watch); err != nil {
		return nil, fmt.Errorf("node store: projections: %w", err)
	}
	return &service{storage: p.storage, es: es, ax: ax}, nil
}

// GetNode returns the node for id via an O(1) keyed lookup. A miss triggers one
// whole-model Replay rebuild and a re-lookup, preserving the lazy self-heal
// after a read-DB loss; only a miss that survives the rebuild is a genuine
// ErrNotFound.
func (s *service) GetNode(
	ctx context.Context,
	id string,
) (domain.Node, error) {
	// An empty id is NOWHERE, never a node — and it must not reach the miss path
	// below, which heals the read model by REPLAYING THE ENTIRE EVENT LOG.
	if id == "" {
		return domain.Node{}, fmt.Errorf("node store: get %q: %w", id, ErrNotFound)
	}
	n, err := s.storage.FindByKey(ctx, id)
	if err != nil {
		return domain.Node{}, err
	}
	if n == nil {
		if n, err = s.healOne(ctx, id); err != nil {
			return domain.Node{}, err
		}
	}
	if n == nil {
		return domain.Node{}, fmt.Errorf("node store: get %q: %w", id, ErrNotFound)
	}
	return *n, nil
}

// healOne rebuilds a SINGLE node's read-model row from the event log and
// returns it, or nil when the id is unknown.
//
// One id, not the whole model: asynx.Replay takes a single aggregate, and that
// is all a keyed GetNode needs. It folds NOTHING when the id is unknown or
// already Forgotten, so a miss for a deleted node (or a stale caller request)
// costs one empty replay instead of folding every OTHER node's entire history
// back through the read model. A genuine whole-model loss still heals lazily,
// one row per GetNode, plus ListByParent rebuilds the lot when the model is
// empty.
func (s *service) healOne(ctx context.Context, id string) (*domain.Node, error) {
	if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
		return nil, fmt.Errorf("node store: heal %q: %w", id, err)
	}
	return s.storage.FindByKey(ctx, id)
}

// ListByParent returns every node whose ParentID is parentID, healing the read
// model first when empty.
func (s *service) ListByParent(
	ctx context.Context,
	parentID string,
) ([]domain.Node, error) {
	all, err := s.allHealed(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Node, 0, len(all))
	for _, n := range all {
		if n.ParentID == parentID {
			result = append(result, n)
		}
	}
	return result, nil
}

// ListAll returns every node, healing the read model first when empty.
func (s *service) ListAll(
	ctx context.Context,
) ([]domain.Node, error) {
	return s.allHealed(ctx)
}

// allHealed returns every row in the durable read model, first healing it via
// whole-model lazy Replay when the model is empty but the event log still holds
// aggregates (mirrors agentchat's allHealed). Every exported read funnels
// through this so a read-DB loss is repaired regardless of which method
// observes it first.
func (s *service) allHealed(
	ctx context.Context,
) ([]domain.Node, error) {
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

// rebuild enumerates every aggregate id in the event log and Replays each into
// the read model. It is a no-op when the event store cannot enumerate its ids or
// when the log is empty (nothing to Replay), so an empty model over an empty log
// stays empty.
//
// A single aggregate that fails to Replay is logged and SKIPPED, not
// propagated: every other aggregate must still heal.
func (s *service) rebuild(
	ctx context.Context,
) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("node store: enumerate aggregate ids: %w", err)
	}
	for _, key := range keys {
		id, ok := strings.CutPrefix(key, eventKeyPrefix)
		if !ok {
			// A non-event key (e.g. a "snapshots:<id>" row for the same aggregate)
			// — skip it; the aggregate is rebuilt from its "events:" key.
			continue
		}
		if err := s.ax.Replay(ctx, id, 1, 0, s.foldReplayed); err != nil {
			slog.ErrorContext(ctx, "node store: replay skipped unreplayable aggregate",
				"id", id, "err", err)
			continue
		}
	}
	return nil
}

// foldReplayed persists each replayed aggregate into the read model. Replay
// folds the event stream, so evt.Aggregate carries the aggregate's state at that
// version; the final event leaves the current state durable.
func (s *service) foldReplayed(
	ctx context.Context,
	evt asynxModels.Event[domain.Node],
) {
	if err := s.storage.Save(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "node store: replay fold", "id", evt.Aggregate.ID, "err", err)
	}
}

// Subscriber registers p as the instance's durable subscriber: a failed save is
// retried by asynx, and parks the aggregate, instead of being logged and
// dropped. The Forget marker deletes the row; a missing row is no error.
func Subscriber(
	p *Projection,
) func(*asynx.Builder[domain.Node]) {
	return func(b *asynx.Builder[domain.Node]) {
		b.WithSubscriber(asynx.Topic("node.*"), p.handle)
	}
}

// Option configures the store projection.
type Option func(*Projection)

// WithPlacementWatch announces every node the projection just created,
// re-parented or deleted. It runs after the write, so a listener re-reading
// the read model sees the new parent.
func WithPlacementWatch(watch func(nodeID string)) Option {
	return func(p *Projection) { p.placement = watch }
}

func (p *Projection) handle(
	ctx context.Context,
	evt asynxModels.Event[domain.Node],
) error {
	if evt.EventName == asynxModels.ForgetEventName {
		if err := p.storage.Delete(ctx, evt.AggregateID); err != nil {
			return fmt.Errorf("node store projection: delete %q: %w", evt.AggregateID, err)
		}
		p.announcePlacement(evt.AggregateID)
		return nil
	}
	if err := p.storage.Save(ctx, evt.Aggregate); err != nil {
		return fmt.Errorf("node store projection: save %q: %w", evt.Aggregate.ID, err)
	}
	if evt.PreviousAggregate.ID == "" || evt.PreviousAggregate.ParentID != evt.Aggregate.ParentID {
		p.announcePlacement(evt.Aggregate.ID)
	}
	return nil
}

func (p *Projection) announcePlacement(
	nodeID string,
) {
	if p.placement != nil {
		p.placement(nodeID)
	}
}
