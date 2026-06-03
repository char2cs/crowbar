package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestProjectCreate_Delegates(t *testing.T) {
	want := domain.Project{ID: "p1", Name: "alpha"}
	repo := &mockProjectRepo{
		createFn: func(_ context.Context, name string) (domain.Project, error) {
			return want, nil
		},
	}
	uc := NewProject(repo)
	got, err := uc.Create(context.Background(), "alpha")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestProjectCreate_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockProjectRepo{
		createFn: func(_ context.Context, name string) (domain.Project, error) {
			return domain.Project{}, boom
		},
	}
	uc := NewProject(repo)
	_, err := uc.Create(context.Background(), "alpha")
	assert.ErrorIs(t, err, boom)
}

func TestProjectGet_Delegates(t *testing.T) {
	want := domain.Project{ID: "p1"}
	repo := &mockProjectRepo{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return want, nil
		},
	}
	uc := NewProject(repo)
	got, err := uc.Get(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestProjectGet_PropagatesError(t *testing.T) {
	repo := &mockProjectRepo{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{}, domain.ErrNotFound
		},
	}
	uc := NewProject(repo)
	_, err := uc.Get(context.Background(), "nope")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestProjectList_Delegates(t *testing.T) {
	want := []domain.Project{{ID: "p1"}, {ID: "p2"}}
	repo := &mockProjectRepo{
		listFn: func(_ context.Context) ([]domain.Project, error) {
			return want, nil
		},
	}
	uc := NewProject(repo)
	got, err := uc.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestProjectList_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockProjectRepo{
		listFn: func(_ context.Context) ([]domain.Project, error) {
			return nil, boom
		},
	}
	uc := NewProject(repo)
	_, err := uc.List(context.Background())
	assert.ErrorIs(t, err, boom)
}

func TestProjectDelete_Delegates(t *testing.T) {
	var called string
	repo := &mockProjectRepo{
		deleteFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewProject(repo)
	require.NoError(t, uc.Delete(context.Background(), "p1"))
	assert.Equal(t, "p1", called)
}

func TestProjectDelete_PropagatesError(t *testing.T) {
	repo := &mockProjectRepo{
		deleteFn: func(_ context.Context, id string) error {
			return domain.ErrNotFound
		},
	}
	uc := NewProject(repo)
	assert.ErrorIs(t, uc.Delete(context.Background(), "nope"), domain.ErrNotFound)
}

// ---------- mock Project repo ----------

type mockProjectRepo struct {
	createFn func(ctx context.Context, name string) (domain.Project, error)
	listFn   func(ctx context.Context) ([]domain.Project, error)
	getFn    func(ctx context.Context, id string) (domain.Project, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockProjectRepo) Create(ctx context.Context, name string) (domain.Project, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name)
	}
	return domain.Project{}, nil
}

func (m *mockProjectRepo) List(ctx context.Context) ([]domain.Project, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockProjectRepo) Get(ctx context.Context, id string) (domain.Project, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return domain.Project{}, nil
}

func (m *mockProjectRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
