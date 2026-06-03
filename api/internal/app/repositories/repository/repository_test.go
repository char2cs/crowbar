package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func openTestDB(t *testing.T) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// openMigratedThenClosedDB opens a store, runs New so the schema exists,
// then immediately closes the underlying connection so subsequent DB calls fail.
func openMigratedThenClosedDB(t *testing.T) Repository {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	r, err := New(store.DB())
	require.NoError(t, err)
	require.NoError(t, store.Close())
	return r
}

func TestNew_AutoMigrate(t *testing.T) {
	store := openTestDB(t)
	_, err := New(store.DB())
	require.NoError(t, err)
}

func TestNew_MigrateFails(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, store.Close()) // close before New so AutoMigrate fails
	_, err = New(store.DB())
	require.Error(t, err)
}

func TestCreate_DBError(t *testing.T) {
	r := openMigratedThenClosedDB(t)
	ctx := context.Background()
	_, err := r.Create(ctx, "proj-1", "repo", "/path")
	require.Error(t, err)
}

func TestList_DBError(t *testing.T) {
	r := openMigratedThenClosedDB(t)
	ctx := context.Background()
	_, err := r.List(ctx, "proj-1")
	require.Error(t, err)
}

func TestGet_DBError(t *testing.T) {
	r := openMigratedThenClosedDB(t)
	ctx := context.Background()
	_, err := r.Get(ctx, "any-id")
	require.Error(t, err)
}

func TestDelete_DBError(t *testing.T) {
	r := openMigratedThenClosedDB(t)
	ctx := context.Background()
	err := r.Delete(ctx, "any-id")
	require.Error(t, err)
}

func TestCreate(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	rec, err := r.Create(ctx, "proj-1", "my-repo", "/tmp/my-repo")
	require.NoError(t, err)
	require.NotEmpty(t, rec.ID)
	require.Equal(t, "proj-1", rec.ProjectID)
	require.Equal(t, "my-repo", rec.Name)
	require.Equal(t, "/tmp/my-repo", rec.Path)
	require.False(t, rec.CreatedAt.IsZero())
}

func TestList_Empty(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	recs, err := r.List(ctx, "proj-1")
	require.NoError(t, err)
	require.Empty(t, recs)
}

func TestList_FiltersByProjectID(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()

	// Create repos under two different projects
	_, err = r.Create(ctx, "proj-A", "repo-A1", "/a1")
	require.NoError(t, err)
	_, err = r.Create(ctx, "proj-A", "repo-A2", "/a2")
	require.NoError(t, err)
	_, err = r.Create(ctx, "proj-B", "repo-B1", "/b1")
	require.NoError(t, err)

	// proj-A should return exactly 2
	recsA, err := r.List(ctx, "proj-A")
	require.NoError(t, err)
	require.Len(t, recsA, 2)

	// proj-B should return exactly 1
	recsB, err := r.List(ctx, "proj-B")
	require.NoError(t, err)
	require.Len(t, recsB, 1)
	require.Equal(t, "repo-B1", recsB[0].Name)

	// unknown project should return empty
	recsC, err := r.List(ctx, "proj-C")
	require.NoError(t, err)
	require.Empty(t, recsC)
}

func TestList_OrderedByCreatedAt(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	first, err := r.Create(ctx, "proj-1", "first", "/first")
	require.NoError(t, err)
	second, err := r.Create(ctx, "proj-1", "second", "/second")
	require.NoError(t, err)

	recs, err := r.List(ctx, "proj-1")
	require.NoError(t, err)
	require.Len(t, recs, 2)
	require.Equal(t, first.ID, recs[0].ID)
	require.Equal(t, second.ID, recs[1].ID)
}

func TestGet_Found(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	created, err := r.Create(ctx, "proj-1", "find-me", "/find-me")
	require.NoError(t, err)

	got, err := r.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "find-me", got.Name)
}

func TestGet_NotFound(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = r.Get(ctx, "nonexistent-id")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete_Found(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	created, err := r.Create(ctx, "proj-1", "to-delete", "/to-delete")
	require.NoError(t, err)

	err = r.Delete(ctx, created.ID)
	require.NoError(t, err)

	_, err = r.Get(ctx, created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Delete(ctx, "nonexistent-id")
	require.ErrorIs(t, err, domain.ErrNotFound)
}
