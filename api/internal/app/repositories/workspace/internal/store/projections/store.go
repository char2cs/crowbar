// Package projections holds the workspace read-side projections that the
// singleton axWorkspace fans events into. store.go is the SAVE-ONLY durable
// projection (spec §3.5 delivery, decision 5): it folds evt.Aggregate into the
// durable read model at state/store/workspace.db and, on Forget, deletes the
// row. Once a hub is registered (hub.go) it announces each event after saving
// it, so no frame ever reaches a client before the read model holds it.
package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
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
	// the save that persists that id's "deleted" row — and, once announcements
	// are expected, by the hub frame that announces it.
	mu               sync.Mutex
	tombstoneWaiters map[string][]chan struct{}
	// expectAnnouncements is set once a hub projection is registered: from then
	// on a tombstone is ready for its purge only once it is both persisted and
	// ANNOUNCED — its frame broadcast. The frame is addressed through the
	// workspace's owning chat, which the purge deletes; a purge that ran first
	// left the client holding a ghost row it was never told had gone.
	expectAnnouncements bool
	announced           map[string]struct{}
	// announce broadcasts a saved event's frame; nil until RegisterHub.
	announce   atomic.Pointer[func(ctx context.Context, ws domain.Workspace)]
	unreadable serialize.Unreadable
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
	return &Store{
		inner:            inner,
		tombstoneWaiters: map[string][]chan struct{}{},
		announced:        map[string]struct{}{},
	}, nil
}

// ExpectAnnouncements makes AwaitTombstone also wait for each tombstone's hub
// frame (see Store.expectAnnouncements). RegisterHub calls it.
func (s *Store) ExpectAnnouncements() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectAnnouncements = true
}

// Announced records that ws's tombstone frame has been broadcast.
func (s *Store) Announced(
	ws domain.Workspace,
) {
	if ws.Status != domain.WorkspaceStatusDeleted {
		return
	}
	s.mu.Lock()
	s.announced[ws.ID] = struct{}{}
	s.mu.Unlock()
	s.wakeTombstoneWaiters(ws.ID)
}

// takeAnnouncement reports whether id's tombstone may be purged as far as the
// hub is concerned, consuming the record.
func (s *Store) takeAnnouncement(
	id string,
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.expectAnnouncements {
		return true
	}
	if _, ok := s.announced[id]; !ok {
		return false
	}
	delete(s.announced, id)
	return true
}

// AwaitTombstone blocks until the read model holds id's persisted "deleted" row
// — and, with a hub registered, until that tombstone's frame is broadcast — and
// returns it, or until ctx ends. It is woken by the save and the broadcast —
// no polling: the waiter registers BEFORE it reads, so a wake that lands
// between the read and the wait is not lost.
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
		if ws != nil && ws.Status == domain.WorkspaceStatusDeleted && s.takeAnnouncement(id) {
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
			s.unreadable.Skip(ctx, "workspace", row.ID, err)
			continue
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
	s.mu.Lock()
	delete(s.announced, id)
	s.mu.Unlock()
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

// Subscriber registers st as a durable subscriber on the workspace builder: a
// failed save is retried by asynx, and parks the aggregate, instead of being
// logged and dropped. The Forget marker deletes the row; a missing row is no error.
func Subscriber(
	st *Store,
) func(*asynx.Builder[domain.Workspace]) {
	return func(b *asynx.Builder[domain.Workspace]) {
		b.WithSubscriber(asynx.Topic("workspace.*"), st.handle)
	}
}

func (s *Store) handle(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) error {
	if evt.EventName == asynxModels.ForgetEventName {
		if err := s.delete(ctx, evt.AggregateID); err != nil {
			return fmt.Errorf("workspace store projection: delete %q: %w", evt.AggregateID, err)
		}
		return nil
	}
	if err := s.save(ctx, evt.Aggregate); err != nil {
		return fmt.Errorf("workspace store projection: save %q: %w", evt.Aggregate.ID, err)
	}
	if announce := s.announce.Load(); announce != nil {
		(*announce)(ctx, evt.Aggregate)
	}
	return nil
}
