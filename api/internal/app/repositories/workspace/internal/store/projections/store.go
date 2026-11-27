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

type workspaceRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Data []byte `gorm:"column:data"`
}

func (workspaceRow) TableName() string {
	return "read_workspaces"
}

type Store struct {
	inner interface {
		Save(ctx context.Context, row workspaceRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*workspaceRow, error)
		FindAll(ctx context.Context) ([]workspaceRow, error)
	}
	mu                  sync.Mutex
	tombstoneWaiters    map[string][]chan struct{}
	expectAnnouncements bool
	announced           map[string]struct{}
	announce            atomic.Pointer[func(ctx context.Context, ws domain.Workspace)]
	unreadable          serialize.Unreadable
}

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

func (s *Store) ExpectAnnouncements() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectAnnouncements = true
}

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

func (s *Store) List(ctx context.Context, fromLog func(context.Context, string) (*domain.Workspace, error)) ([]domain.Workspace, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace store projection: find all: %w", err)
	}
	result := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		ws, err := unmarshalWorkspace(row.Data)
		if err != nil {
			s.unreadable.Skip(ctx, "workspace", row.ID, err)
			ws, err = fromLog(ctx, row.ID)
		}
		if err != nil {
			return nil, err
		}
		if ws != nil {
			result = append(result, *ws)
		}
	}
	return result, nil
}

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
