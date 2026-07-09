package wspaths_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
)

func newStore(
	t *testing.T,
) (context.Context, wspaths.WorkspacePaths) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := wspaths.NewWorkspacePaths(db)
	require.NoError(t, err)
	return context.Background(), s
}

func TestWorkspacePaths_CRUD(t *testing.T) {
	ctx, s := newStore(t)

	require.NoError(t, s.Put(ctx, "ws-1", "/h/projects/p/github.com/o/r/main"))

	got, err := s.Get(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, "/h/projects/p/github.com/o/r/main", got)

	require.NoError(t, s.Delete(ctx, "ws-1"))

	_, err = s.Get(ctx, "ws-1")
	assert.ErrorIs(t, err, wspaths.ErrNotFound)
}

func TestWorkspacePaths_Get_NotFound(t *testing.T) {
	ctx, s := newStore(t)

	_, err := s.Get(ctx, "missing")
	assert.ErrorIs(t, err, wspaths.ErrNotFound)
}

func TestWorkspacePaths_Put_Upserts(t *testing.T) {
	ctx, s := newStore(t)

	require.NoError(t, s.Put(ctx, "ws-1", "/old/path"))
	require.NoError(t, s.Put(ctx, "ws-1", "/new/path"))

	got, err := s.Get(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, "/new/path", got)
}

func TestWorkspacePaths_Delete_Idempotent(t *testing.T) {
	ctx, s := newStore(t)

	assert.NoError(t, s.Delete(ctx, "missing"))
}

func TestNewWorkspacePaths_ClosedDBErrors(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = wspaths.NewWorkspacePaths(db)
	assert.Error(t, err)
}
