package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// heldStore is a store whose Save of the held key lands, then waits for
// release before it returns: the window a later write can overtake it in.
type heldStore[T any] struct {
	mu      sync.Mutex
	rows    map[string]T
	key     func(T) string
	held    string
	landed  chan struct{}
	release chan struct{}
}

func newHeldStore[T any](key func(T) string, held string) *heldStore[T] {
	return &heldStore[T]{
		rows: map[string]T{}, key: key, held: held,
		landed: make(chan struct{}), release: make(chan struct{}),
	}
}

func (s *heldStore[T]) Save(_ context.Context, item T) error {
	s.mu.Lock()
	s.rows[s.key(item)] = item
	s.mu.Unlock()
	if s.key(item) == s.held {
		s.held = ""
		close(s.landed)
		<-s.release
	}
	return nil
}

func (s *heldStore[T]) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, id)
	return nil
}

func (s *heldStore[T]) FindByKey(_ context.Context, id string) (*T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (s *heldStore[T]) FindAll(context.Context) ([]T, error) { return nil, nil }

func (s *heldStore[T]) FindWhere(context.Context, T) ([]T, error) { return nil, nil }

// The last frame a client hears for a row is the row the store last saved,
// though the write that saved it returns before an earlier one.
func TestPublishingStores_AnOvertakenSaveDoesNotRepublishItsStaleRow(t *testing.T) {
	ctx := context.Background()
	lastName := func(out chan domainstream.Frame, n int) string {
		var name string
		for range n {
			var row struct {
				Name string `json:"name"`
			}
			require.NoError(t, json.Unmarshal((<-out).Data, &row))
			name = row.Name
		}
		return name
	}

	t.Run("project", func(t *testing.T) {
		late, out := holdProjects(t)
		rows := newHeldStore(func(p domain.Project) string { return p.ID }, "p1")
		projects := &publishingProjects{Store: rows, late: late}
		saveOld := make(chan struct{})
		go func() {
			defer close(saveOld)
			_ = projects.Save(ctx, domain.Project{ID: "p1", Name: "old"})
		}()
		<-rows.landed
		require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1", Name: "new"}))
		close(rows.release)
		<-saveOld
		assert.Equal(t, "new", lastName(out, 2))
	})

	t.Run("repo", func(t *testing.T) {
		late, out := holdProjects(t)
		rows := newHeldStore(func(r domain.Repository) string { return r.ID }, "r1")
		at := func(context.Context, string) (tree.Node, error) { return tree.Node{}, nil }
		repos := publishingRepos{ScopedStore: rows, entities: &repoEntities{
			late: late, rows: rows, placements: dto.NewPlacements(at, nil),
		}}
		saveOld := make(chan struct{})
		go func() {
			defer close(saveOld)
			_ = repos.Save(ctx, domain.Repository{ID: "r1", Name: "old"})
		}()
		<-rows.landed
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", Name: "new"}))
		close(rows.release)
		<-saveOld
		assert.Equal(t, "new", lastName(out, 2))
	})
}
