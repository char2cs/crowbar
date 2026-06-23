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

func (e *errInner) Save(_ context.Context, _ chatRow) error  { return e.err }
func (e *errInner) Delete(_ context.Context, _ string) error { return e.err }
func (e *errInner) FindByKey(_ context.Context, _ string) (*chatRow, error) {
	return nil, e.err
}
func (e *errInner) FindAll(_ context.Context) ([]chatRow, error) { return nil, e.err }

// badDataInner returns a row with corrupt JSON data.
type badDataInner struct{}

func (b *badDataInner) Save(_ context.Context, _ chatRow) error  { return nil }
func (b *badDataInner) Delete(_ context.Context, _ string) error { return nil }
func (b *badDataInner) FindByKey(_ context.Context, _ string) (*chatRow, error) {
	return &chatRow{ID: "x", Data: []byte("not-json")}, nil
}

func (b *badDataInner) FindAll(_ context.Context) ([]chatRow, error) {
	return []chatRow{{ID: "x", Data: []byte("not-json")}}, nil
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
	c := domain.Chat{ID: "c1", WsID: "w1", Title: "hi", CreatedAt: time.Unix(1, 0).UTC()}
	require.NoError(t, st.Save(ctx, c))

	got, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "hi", got.Title)

	all, err := st.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, st.Delete(ctx, "c1"))
	got, err = st.FindByKey(ctx, "c1")
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
	_, err := st.FindByKey(ctx, "c1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat storage: find")
}

func TestStorage_FindByKey_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindByKey(ctx, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat storage: unmarshal")
}

func TestStorage_FindAll_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat storage: find all")
}

func TestStorage_FindAll_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat storage: unmarshal")
}

func TestStorage_Save_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	err := st.Save(ctx, domain.Chat{ID: "c1"})
	require.Error(t, err)
}

func TestUnmarshalChat_BadJSON(t *testing.T) {
	_, err := unmarshalChat([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat storage: unmarshal")
}

func TestStorage_NewStorageStore_BadDB(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
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

	_, err = New(context.Background(), db, nil, func(domain.Chat) {}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat store")
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	_, err = New(context.Background(), db, ax, func(domain.Chat) {}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat store: projections")
}

func TestStoreService_ListByWorkspace_StorageError(t *testing.T) {
	ctx := context.Background()
	svc := &storeService{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	_, err := svc.ListByWorkspace(ctx, "w1")
	require.Error(t, err)
}
