// Package projections holds the workspace read-side projections that the
// singleton axWorkspace fans events into. store.go is the SAVE-ONLY durable
// projection (spec §3.5 delivery, decision 5): it folds evt.Aggregate into the
// durable read model at state/store/workspace.db and, on Forget, deletes the
// row. The distinct hub projection (hub.go, Task 6) owns WS fan-out, so the
// durable read model and the broadcast frame derive independently from
// evt.Aggregate and cannot drift.
package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// workspaceRow is the durable read-model row persisted to state/store/workspace.db.
type workspaceRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Data []byte `gorm:"column:data"`
}

func (workspaceRow) TableName() string {
	return "read_workspaces"
}

// Store is the durable workspace read model (spec §3.7): a projected, queryable
// view of the aggregate persisted to state/store/workspace.db. Each row carries
// project_id/repo_id off the folded aggregate, so the read model doubles as the
// location index — no separate location table is needed.
type Store struct {
	inner interface {
		Save(ctx context.Context, row workspaceRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*workspaceRow, error)
		FindAll(ctx context.Context) ([]workspaceRow, error)
	}
	// tombstoneWaiters are the AwaitTombstone callers parked on an id, woken by
	// the save that persists that id's "deleted" row.
	mu               sync.Mutex
	tombstoneWaiters map[string][]chan struct{}
}

// NewStore builds the durable read-model store over the read-model DB
// (state/store/workspace.db), auto-migrating the read_workspaces table.
func NewStore(
	db *gormdb.DB,
) (*Store, error) {
	inner, err := storesqlite.NewFromDB[workspaceRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("workspace store projection: %w", err)
	}
	return &Store{inner: inner, tombstoneWaiters: map[string][]chan struct{}{}}, nil
}

// AwaitTombstone blocks until the read model holds id's persisted "deleted" row
// and returns it, or until ctx ends. It is woken by the save that persists the
// tombstone — no polling: the waiter registers BEFORE it reads, so a save that
// lands between the read and the wait still wakes it.
func (s *Store) AwaitTombstone(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	wake := make(chan struct{}, 1)
	s.mu.Lock()
	s.tombstoneWaiters[id] = append(s.tombstoneWaiters[id], wake)
	s.mu.Unlock()
	defer s.stopWaiting(id, wake)
	for {
		ws, err := s.Get(ctx, id)
		if err != nil {
			return domain.Workspace{}, err
		}
		if ws != nil && ws.Status == domain.WorkspaceStatusDeleted {
			return *ws, nil
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return domain.Workspace{}, ctx.Err()
		}
	}
}

func (s *Store) stopWaiting(
	id string,
	wake chan struct{},
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	waiters := s.tombstoneWaiters[id]
	for i, w := range waiters {
		if w == wake {
			waiters = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(waiters) == 0 {
		delete(s.tombstoneWaiters, id)
		return
	}
	s.tombstoneWaiters[id] = waiters
}

func (s *Store) wakeTombstoneWaiters(
	id string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, wake := range s.tombstoneWaiters[id] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// List returns every workspace currently in the read model.
func (s *Store) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace store projection: find all: %w", err)
	}
	result := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		ws, err := unmarshalWorkspace(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *ws)
	}
	return result, nil
}

// Get returns the workspace with the given id, or nil if the read model has no
// such row.
func (s *Store) Get(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workspace store projection: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalWorkspace(row.Data)
}

// Fold persists ws into the durable read model — the write half of this
// save-only projection, exported so the lazy Replay repair (store.ListOrRebuild,
// spec §3.7) can fold each replayed aggregate back into state/store/workspace.db.
// Live events reach the model through the registered onEvent subscription instead;
// this seam exists only for the on-demand whole-model rebuild.
func (s *Store) Fold(
	ctx context.Context,
	ws domain.Workspace,
) error {
	return s.save(ctx, ws)
}

func (s *Store) save(
	ctx context.Context,
	ws domain.Workspace,
) error {
	data, err := json.Marshal(ws)
	if err != nil {
		return fmt.Errorf("workspace store projection: marshal: %w", err)
	}
	if err := s.inner.Save(ctx, workspaceRow{ID: ws.ID, Data: data}); err != nil {
		return err
	}
	if ws.Status == domain.WorkspaceStatusDeleted {
		s.wakeTombstoneWaiters(ws.ID)
	}
	return nil
}

func (s *Store) delete(
	ctx context.Context,
	id string,
) error {
	return s.inner.Delete(ctx, id)
}

// Drop deletes id's row directly — for a tombstone whose aggregate is already
// Forgotten, which no projection event will ever reach again.
func (s *Store) Drop(
	ctx context.Context,
	id string,
) error {
	return s.delete(ctx, id)
}

func unmarshalWorkspace(
	data []byte,
) (*domain.Workspace, error) {
	var ws domain.Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("workspace store projection: unmarshal: %w", err)
	}
	return &ws, nil
}

// RegisterStore subscribes the SAVE-ONLY read-model projection to every workspace
// event on the singleton axWorkspace: it folds evt.Aggregate into the durable
// read model and, on Forget, deletes the aggregate's row (a cheap, synchronous
// OnForget row-delete — spec §3.6, no fs/git/network io). Unlike the retired
// combined projector it does NOT broadcast; the hub projection owns fan-out
// (decision 5). It is designed to register ONCE on the singleton, not per
// aggregate.
func RegisterStore(
	st *Store,
	ax asynx.Asynx[domain.Workspace],
) error {
	p := &storeProjector{store: st}
	if _, err := ax.Subscribe(asynx.Topic("workspace.*"), p.onEvent); err != nil {
		return fmt.Errorf("workspace store projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("workspace store projection: on forget: %w", err)
	}
	return nil
}

type storeProjector struct {
	store *Store
}

func (p *storeProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	if err := p.saveWithRetry(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "workspace store projection: save", "id", evt.Aggregate.ID, "err", err)
	}
}

func (p *storeProjector) saveWithRetry(
	ctx context.Context,
	ws domain.Workspace,
) error {
	var err error
	for range 3 {
		if err = p.store.save(ctx, ws); err == nil {
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
	evt asynxModels.Event[domain.Workspace],
) {
	if err := p.store.delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "workspace store projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
