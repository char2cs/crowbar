// Package store owns the workspace read model (state/store/workspace.db), fed by axWorkspace's durable
// store projection. Open does no reconcile: ListOrRebuild heals a lost model lazily via Replay, while the
// raw List never replays; the hub projection derives independently, so the two cannot drift.
package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the workspace read model. Per-id reads fold the event log via axWorkspace.Get, except Get
// below, which the delete reactor's ordering gate needs because it reflects only the DURABLE projection.
type Store interface {
	// List returns the durable projection at state/store/workspace.db DIRECTLY,
	// WITHOUT lazy Replay repair. The boot orphan-sweep uses this path
	// so boot pays no replay and a lost model reaps nothing.
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// Get returns the DURABLE read-model row, or nil. The delete reactor purges only once the projection has
	// persisted the tombstone, so it can never be re-saved after the worktree is gone. Also the reactor's
	// StoreReader seam.
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
	// ListOrRebuild returns the read model (also the location index), first healing it via lazy Replay when
	// it is empty but the event log still holds aggregates.
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
// ax wraps, kept so ListOrRebuild can enumerate its aggregates.
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
	return s.store.List(ctx, func(context.Context, string) (*domain.Workspace, error) { return nil, nil })
}

// Get returns the durable read-model row for id (nil when absent), delegating to
// the save-only store projection — the persisted view the delete reactor's
// ordering gate observes.
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
