package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// errInner is a fake inner store that always errors.
type errInner struct{ err error }

func (e *errInner) Save(_ context.Context, _ workspaceRow) error { return e.err }
func (e *errInner) Delete(_ context.Context, _ string) error     { return e.err }
func (e *errInner) FindByKey(_ context.Context, _ string) (*workspaceRow, error) {
	return nil, e.err
}
func (e *errInner) FindAll(_ context.Context) ([]workspaceRow, error) { return nil, e.err }

// badDataInner returns a row with corrupt JSON data.
type badDataInner struct{}

func (b *badDataInner) Save(_ context.Context, _ workspaceRow) error { return nil }
func (b *badDataInner) Delete(_ context.Context, _ string) error     { return nil }
func (b *badDataInner) FindByKey(_ context.Context, _ string) (*workspaceRow, error) {
	return &workspaceRow{ID: "x", Data: []byte("not-json")}, nil
}

func (b *badDataInner) FindAll(_ context.Context) ([]workspaceRow, error) {
	return []workspaceRow{{ID: "x", Data: []byte("not-json")}}, nil
}

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

func TestStorage_FindByKey_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	_, err := st.FindByKey(ctx, "w1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace storage: find")
}

func TestStorage_FindByKey_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindByKey(ctx, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace storage: unmarshal")
}

func TestStorage_FindAll_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace storage: find all")
}

func TestStorage_FindAll_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace storage: unmarshal")
}

func TestStorage_Save_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	err := st.Save(ctx, domain.Workspace{ID: "w1"})
	require.Error(t, err)
}

func TestUnmarshalWorkspace_BadJSON(t *testing.T) {
	_, err := unmarshalWorkspace([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace storage: unmarshal")
}

func TestStorage_NewStorageStore_BadDB(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	// Close the underlying connection so AutoMigrate fails.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = newStorageStore(db)
	require.Error(t, err)
}

func TestNew_StorageError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// New should surface the storage init error.
	_, err = New(db, nil, func(domain.Workspace) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace store")
}
