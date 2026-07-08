// Package store owns the workspace read model: the durable, queryable projection
// of the aggregate at state/store/workspace.db. New builds the read model over
// the read-pool DB and registers the SAVE-ONLY store projection on the singleton
// axWorkspace (spec §3.5/§3.7, decision 5). Unlike the retired per-entity store
// it does NO eager reconcile on open — a normal boot re-opens the durable read
// model with zero replay; read-model repair is lazy (Tasks 9/11). The hub
// projection is registered separately by repositories.Container, which owns the
// enrichment callback, so the durable read model and the WS frame derive
// independently from evt.Aggregate and cannot drift.
package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the workspace read model. List reads the durable projection at
// state/store/workspace.db directly (it doubles as the location index, §3.7);
// per-id reads fold the aggregate from the event log via axWorkspace.Get, so Get
// is not part of this surface.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

// New builds the durable read model over db (state/store/workspace.db) and
// registers the save-only store projection on ax, once, for the singleton
// axWorkspace. It performs no reconcile: read-model repair is lazy (Tasks 9/11).
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.Workspace],
) (Store, error) {
	st, err := projections.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	if err := projections.RegisterStore(st, ax); err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	return st, nil
}
