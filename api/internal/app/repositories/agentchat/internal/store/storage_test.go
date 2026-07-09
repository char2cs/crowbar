package store

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// errInner is a fake inner store that always errors.
type errInner struct{ err error }

func (e *errInner) Save(_ context.Context, _ agentChatRow) error { return e.err }
func (e *errInner) Delete(_ context.Context, _ string) error     { return e.err }
func (e *errInner) FindByKey(_ context.Context, _ string) (*agentChatRow, error) {
	return nil, e.err
}
func (e *errInner) FindAll(_ context.Context) ([]agentChatRow, error) { return nil, e.err }

// badDataInner returns a row with corrupt JSON data.
type badDataInner struct{}

func (b *badDataInner) Save(_ context.Context, _ agentChatRow) error { return nil }
func (b *badDataInner) Delete(_ context.Context, _ string) error     { return nil }
func (b *badDataInner) FindByKey(_ context.Context, _ string) (*agentChatRow, error) {
	return &agentChatRow{ID: "x", Data: []byte("not-json")}, nil
}

func (b *badDataInner) FindAll(_ context.Context) ([]agentChatRow, error) {
	return []agentChatRow{{ID: "x", Data: []byte("not-json")}}, nil
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
	chat := domain.AgentChat{
		ID:          "c1",
		WorkspaceID: "w1",
		Status:      domain.AgentChatStatusActive,
	}
	require.NoError(t, st.Save(ctx, chat))

	got, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "w1", got.WorkspaceID)

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
	assert.Contains(t, err.Error(), "agentchat storage: find")
}

func TestStorage_FindByKey_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindByKey(ctx, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat storage: unmarshal")
}

func TestStorage_FindAll_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat storage: find all")
}

func TestStorage_FindAll_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat storage: unmarshal")
}

func TestStorage_Save_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	err := st.Save(ctx, domain.AgentChat{ID: "c1"})
	require.Error(t, err)
}

func TestStorage_Delete_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	err := st.Delete(ctx, "c1")
	require.Error(t, err)
}

func TestUnmarshalAgentChat_BadJSON(t *testing.T) {
	_, err := unmarshalAgentChat([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat storage: unmarshal")
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

func TestIsTransientIOError(t *testing.T) {
	assert.True(t, isTransientIOError(errors.New("disk I/O error")))
	assert.False(t, isTransientIOError(errors.New("some other error")))
}
