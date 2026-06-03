package project

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

// openClosedDB returns a GORM DB whose underlying sql.DB has been closed,
// so any operation against it returns an error — used to exercise error paths.
func openClosedDB(t *testing.T) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "closed.db"))
	require.NoError(t, err)
	require.NoError(t, store.Close()) // close immediately
	return store
}

func TestNew_AutoMigrate(t *testing.T) {
	store := openTestDB(t)
	_, err := New(store.DB())
	require.NoError(t, err)
}

func TestCreate(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	p, err := r.Create(ctx, "my-project")
	require.NoError(t, err)
	require.NotEmpty(t, p.ID)
	require.Equal(t, "my-project", p.Name)
	require.False(t, p.CreatedAt.IsZero())
}

func TestList_Empty(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	projects, err := r.List(ctx)
	require.NoError(t, err)
	require.Empty(t, projects)
}

func TestList_Multiple(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = r.Create(ctx, "alpha")
	require.NoError(t, err)
	_, err = r.Create(ctx, "beta")
	require.NoError(t, err)
	_, err = r.Create(ctx, "gamma")
	require.NoError(t, err)

	projects, err := r.List(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 3)
}

func TestGet_Found(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	created, err := r.Create(ctx, "find-me")
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
	created, err := r.Create(ctx, "to-delete")
	require.NoError(t, err)

	err = r.Delete(ctx, created.ID)
	require.NoError(t, err)

	// Verify it's gone
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

func TestNew_MigrateFails(t *testing.T) {
	closedStore := openClosedDB(t)
	_, err := New(closedStore.DB())
	require.Error(t, err)
}

func TestCreate_DBError(t *testing.T) {
	// Create a repo with a working DB, then close the store to make the DB fail.
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	r, err := New(store.DB())
	require.NoError(t, err)
	require.NoError(t, store.Close()) // close AFTER migration so New succeeds

	ctx := context.Background()
	_, err = r.Create(ctx, "fail")
	require.Error(t, err)
}

func TestList_DBError(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	r, err := New(store.DB())
	require.NoError(t, err)
	require.NoError(t, store.Close())

	ctx := context.Background()
	_, err = r.List(ctx)
	require.Error(t, err)
}

func TestGet_DBError(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	r, err := New(store.DB())
	require.NoError(t, err)
	require.NoError(t, store.Close())

	ctx := context.Background()
	_, err = r.Get(ctx, "any-id")
	require.Error(t, err)
}

func TestDelete_DBError(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	r, err := New(store.DB())
	require.NoError(t, err)
	require.NoError(t, store.Close())

	ctx := context.Background()
	err = r.Delete(ctx, "any-id")
	require.Error(t, err)
}

func TestList_OrderedByCreatedAt(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	first, err := r.Create(ctx, "first")
	require.NoError(t, err)
	second, err := r.Create(ctx, "second")
	require.NoError(t, err)

	projects, err := r.List(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 2)
	// Order should be ASC by created_at; IDs match insertion order
	require.Equal(t, first.ID, projects[0].ID)
	require.Equal(t, second.ID, projects[1].ID)
}
