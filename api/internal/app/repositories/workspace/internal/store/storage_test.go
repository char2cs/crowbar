package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStorage(
	t *testing.T,
) (context.Context, storage) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	return context.Background(), st
}

func TestStorage_SaveFindDelete(t *testing.T) {
	ctx, st := newStorage(t)
	ws := domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", CreatedAt: time.Unix(1, 0).UTC()}
	require.NoError(t, st.Save(ctx, ws))

	got, err := st.FindByKey(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "b", got.Branch)

	all, err := st.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, st.Delete(ctx, "w1"))
	got, err = st.FindByKey(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStorage_FindByKey_MissingReturnsNil(t *testing.T) {
	ctx, st := newStorage(t)
	got, err := st.FindByKey(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}
