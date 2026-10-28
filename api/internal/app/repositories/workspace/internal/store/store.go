// Package store owns the workspace read model: the durable, queryable projection
// of the aggregate at state/store/workspace.db, fed by the save-only store
// projection registered as axWorkspace's durable subscriber (decision 5). Unlike the retired per-entity store
// it does NO eager reconcile on open — a normal boot re-opens the durable read
// model with zero replay; read-model repair is lazy (Tasks 9/11): the per-request
// ListOrRebuild heals a lost model on first access via whole-model asynx.Replay,
// while the raw List (the boot orphan-sweep path) never replays. The hub
// projection is registered separately by repositories.Container, which owns the
// enrichment callback, so the durable read model and the WS frame derive
// independently from evt.Aggregate and cannot drift.
package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the workspace read model. Per-request per-id reads fold the aggregate
// from the event log via axWorkspace.Get (always current), so that per-id read is
// not part of this surface. The one exception is Get below, which the async delete
// reactor's ordering gate needs precisely because it reflects only the DURABLE
// projection, not the always-current fold.
type Store interface {
	// List returns the durable projection at state/store/workspace.db DIRECTLY,
	// WITHOUT lazy Replay repair. The boot orphan-sweep uses this path (spec §3.8)
	// so boot pays no replay and a lost model reaps nothing.
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// Get returns the workspace's DURABLE read-model row, or nil when the model has
	// no such row. It reflects only what the store projection has persisted — NOT
	// the always-current event-log fold Workspace.Get returns — which is exactly
	// what the async delete reactor's ordering gate must observe: it purges only
	// once the projection has persisted the "deleted" tombstone, so the tombstone
	// can never be re-saved after the worktree is gone (spec §3.6 ordering
	// contract). Also satisfies the reactor package's StoreReader seam.
	Get(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
	// Drop deletes id's durable row directly. It exists for the one row the
	// projection can no longer delete: a tombstone whose aggregate was already
	// Forgotten before its marker's row delete landed.
	Drop(
		ctx context.Context,
		id string,
	) error
	// AwaitTombstone blocks until the durable projection has persisted id's
	// "deleted" row and returns it — woken by that save, never by polling. It
	// satisfies the delete reactor's StoreReader seam.
	AwaitTombstone(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	// ListOrRebuild returns the read model (which doubles as the location index,
	// §3.7), first healing it via whole-model lazy Replay when the model is empty
	// but the event log still holds aggregates (spec §3.7, decision 7). The
	// per-request workspace List uses this path.
	ListOrRebuild(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

// service is the workspace read model over the durable store projection. It holds
// es (the event log's serialize.AggregateLister source) and ax (for Replay) so
// ListOrRebuild can rebuild a lost read model on demand.
type service struct {
	store *projections.Store
	es    asynxModels.Store
	ax    asynx.Asynx[domain.Workspace]
}

// New builds the workspace read model over st, which the instance ax already
// delivers to as a durable subscriber (projections.Subscriber). es is the log
// ax wraps, kept so ListOrRebuild can enumerate its aggregates (§3.7).
func New(
	st *projections.Store,
	es asynxModels.Store,
	ax asynx.Asynx[domain.Workspace],
) Store {
	return &service{store: st, es: es, ax: ax}
}

// RegisterHub registers the hub (WS fan-out) projection over s's own
// projection store, so a tombstone is purged only once its frame is out (see
// projections.RegisterHub).
func RegisterHub[F any](
	s Store,
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	svc, ok := s.(*service)
	if !ok {
		return fmt.Errorf("workspace store: hub needs the projection store")
	}
	projections.RegisterHub(svc.store, enrich, broadcast)
	return nil
}

// List returns the durable read model directly (no replay).
func (s *service) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return s.store.List(ctx)
}

// Get returns the durable read-model row for id (nil when absent), delegating to
// the save-only store projection — the persisted view the delete reactor's
// ordering gate observes (spec §3.6).
func (s *service) Get(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	return s.store.Get(ctx, id)
}

// AwaitTombstone delegates to the save-only store projection, which wakes the
// waiter from the save that persists the tombstone.
func (s *service) AwaitTombstone(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return s.store.AwaitTombstone(ctx, id)
}

// Drop deletes id's durable read-model row directly.
func (s *service) Drop(
	ctx context.Context,
	id string,
) error {
	return s.store.Drop(ctx, id)
}
