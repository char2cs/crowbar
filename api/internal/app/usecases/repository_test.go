package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRepositoryCreate_Happy(t *testing.T) {
	want := domain.Repository{ID: "r1", ProjectID: "p1", Name: "core", Path: "/tmp/core"}
	projectRepo := &mockProjectRepo{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{ID: id}, nil
		},
	}
	repoRepo := &mockRepositoryRepo{
		createFn: func(_ context.Context, projectID, name, path string) (domain.Repository, error) {
			return want, nil
		},
	}
	uc := NewRepository(repoRepo, projectRepo)
	got, err := uc.Create(context.Background(), "p1", "core", "/tmp/core")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRepositoryCreate_ProjectNotFound(t *testing.T) {
	projectRepo := &mockProjectRepo{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{}, domain.ErrNotFound
		},
	}
	uc := NewRepository(&mockRepositoryRepo{}, projectRepo)
	_, err := uc.Create(context.Background(), "missing", "core", "/tmp")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRepositoryCreate_ProjectCheckError(t *testing.T) {
	boom := errors.New("db down")
	projectRepo := &mockProjectRepo{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{}, boom
		},
	}
	uc := NewRepository(&mockRepositoryRepo{}, projectRepo)
	_, err := uc.Create(context.Background(), "p1", "core", "/tmp")
	assert.ErrorIs(t, err, boom)
}

func TestRepositoryCreate_RepoCreateError(t *testing.T) {
	boom := errors.New("boom")
	projectRepo := &mockProjectRepo{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{ID: id}, nil
		},
	}
	repoRepo := &mockRepositoryRepo{
		createFn: func(_ context.Context, projectID, name, path string) (domain.Repository, error) {
			return domain.Repository{}, boom
		},
	}
	uc := NewRepository(repoRepo, projectRepo)
	_, err := uc.Create(context.Background(), "p1", "core", "/tmp")
	assert.ErrorIs(t, err, boom)
}

func TestRepositoryGet_Delegates(t *testing.T) {
	want := domain.Repository{ID: "r1"}
	repoRepo := &mockRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return want, nil
		},
	}
	uc := NewRepository(repoRepo, &mockProjectRepo{})
	got, err := uc.Get(context.Background(), "r1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRepositoryList_Delegates(t *testing.T) {
	want := []domain.Repository{{ID: "r1"}, {ID: "r2"}}
	repoRepo := &mockRepositoryRepo{
		listFn: func(_ context.Context, projectID string) ([]domain.Repository, error) {
			return want, nil
		},
	}
	uc := NewRepository(repoRepo, &mockProjectRepo{})
	got, err := uc.List(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRepositoryDelete_Delegates(t *testing.T) {
	var called string
	repoRepo := &mockRepositoryRepo{
		deleteFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewRepository(repoRepo, &mockProjectRepo{})
	require.NoError(t, uc.Delete(context.Background(), "r1"))
	assert.Equal(t, "r1", called)
}

// ---------- mock Repository repo ----------

type mockRepositoryRepo struct {
	createFn func(ctx context.Context, projectID, name, path string) (domain.Repository, error)
	listFn   func(ctx context.Context, projectID string) ([]domain.Repository, error)
	getFn    func(ctx context.Context, id string) (domain.Repository, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockRepositoryRepo) Create(ctx context.Context, projectID, name, path string) (domain.Repository, error) {
	if m.createFn != nil {
		return m.createFn(ctx, projectID, name, path)
	}
	return domain.Repository{}, nil
}

func (m *mockRepositoryRepo) List(ctx context.Context, projectID string) ([]domain.Repository, error) {
	if m.listFn != nil {
		return m.listFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockRepositoryRepo) Get(ctx context.Context, id string) (domain.Repository, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return domain.Repository{}, nil
}

func (m *mockRepositoryRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
