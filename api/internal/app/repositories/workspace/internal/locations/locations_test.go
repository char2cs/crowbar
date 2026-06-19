package locations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/locations"
)

func newStore(
	t *testing.T,
) (context.Context, locations.Store) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := locations.New(db)
	require.NoError(t, err)
	return context.Background(), st
}

func TestSaveAndGet(t *testing.T) {
	ctx, st := newStore(t)
	require.NoError(t, st.Save(ctx, locations.Location{ID: "w1", ProjectID: "p1", RepoID: "r1"}))

	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, locations.Location{ID: "w1", ProjectID: "p1", RepoID: "r1"}, got)
}

func TestSaveUpserts(t *testing.T) {
	ctx, st := newStore(t)
	require.NoError(t, st.Save(ctx, locations.Location{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, st.Save(ctx, locations.Location{ID: "w1", ProjectID: "p1", RepoID: "r2"}))

	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "r2", got.RepoID)
}

func TestGet_NotFound(t *testing.T) {
	ctx, st := newStore(t)
	_, err := st.Get(ctx, "missing")
	assert.ErrorIs(t, err, locations.ErrNotFound)
}

func TestList(t *testing.T) {
	ctx, st := newStore(t)
	require.NoError(t, st.Save(ctx, locations.Location{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, st.Save(ctx, locations.Location{ID: "w2", ProjectID: "p1", RepoID: "r2"}))

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDelete(t *testing.T) {
	ctx, st := newStore(t)
	require.NoError(t, st.Save(ctx, locations.Location{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, st.Delete(ctx, "w1"))

	_, err := st.Get(ctx, "w1")
	assert.True(t, errors.Is(err, locations.ErrNotFound))
}

func TestDelete_Idempotent(t *testing.T) {
	ctx, st := newStore(t)
	assert.NoError(t, st.Delete(ctx, "missing"))
}

func TestNew_ClosedDBErrors(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = locations.New(db)
	assert.Error(t, err)
}
