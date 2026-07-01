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

// Store is the workspace read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
}

type storeService struct {
	storage storage
}

// New builds the read-model store, registering the projection that keeps it in
// sync with the aggregate and fans every row out through broadcast, then
// reconciles the read model from the authoritative event store for aggregateID.
//
// The event log and the read model are separate SQLite DBs written without a
// shared transaction, and the projection runs asynchronously after the event is
// durable; a crash (or a dropped projection) in that window leaves the event
// durable but the read-model row stale or missing. Because List reads the read
// model, without reconcile-on-open a workspace would be missing or stale forever
// after a mid-projection crash, while Get (which reads the event store) stays
// correct — a permanent divergence. The reconcile is best-effort: Get reads the
// event store directly, so a failed reconcile never blocks opening the entity,
// and the next write self-corrects the row.
func New(
	ctx context.Context,
	db *gormdb.DB,
	ax asynx.Asynx[domain.Workspace],
	broadcast BroadcastFunc,
	aggregateID string,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("workspace store: projections: %w", err)
	}
	svc := &storeService{storage: st}
	if rErr := svc.reconcile(ctx, ax, aggregateID); rErr != nil {
		slog.WarnContext(ctx, "workspace store: read-model reconcile failed; self-corrects on next write",
			"aggregate", aggregateID, "err", rErr)
	}
	return svc, nil
}

// reconcile overwrites the read-model row for aggregateID with the current state
// replayed from the event store. A not-yet-created aggregate (no events) is a
// no-op, so this is safe to run on the open of a brand-new workspace.
func (s *storeService) reconcile(
	ctx context.Context,
	ax asynx.Asynx[domain.Workspace],
	aggregateID string,
) error {
	ws, err := ax.Get(ctx, aggregateID)
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil
		}
		return err
	}
	if ws.ID == "" {
		return nil
	}
	return s.storage.Save(ctx, ws)
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	return s.storage.FindByKey(ctx, id)
}
