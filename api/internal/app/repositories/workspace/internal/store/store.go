// Package store owns the workspace read model: the durable, queryable projection
// of the aggregate at state/store/workspace.db. New builds the read model over
// the read-pool DB and registers the SAVE-ONLY store projection on the singleton
// axWorkspace (spec §3.5/§3.7, decision 5). Unlike the retired per-entity store
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
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the workspace read model. Per-id reads fold the aggregate from the
// event log via axWorkspace.Get, so Get is not part of this surface.
type Store interface {
	// List returns the durable projection at state/store/workspace.db DIRECTLY,
	// WITHOUT lazy Replay repair. The boot orphan-sweep uses this path (spec §3.8)
	// so boot pays no replay and a lost model reaps nothing.
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
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

// New builds the durable read model over db (state/store/workspace.db) and
// registers the save-only store projection on ax, once, for the singleton
// axWorkspace. It performs no reconcile: read-model repair is lazy (Tasks 9/11).
// es is the same per-type event log ax wraps, retained so ListOrRebuild can reach
// its serialize.AggregateLister capability for the whole-model rebuild (§3.7).
func New(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.Workspace],
) (Store, error) {
	st, err := projections.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	if err := projections.RegisterStore(st, ax); err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	return &service{store: st, es: es, ax: ax}, nil
}

// List returns the durable read model directly (no replay).
func (s *service) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return s.store.List(ctx)
}
