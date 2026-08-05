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

func (e *errInner) Save(_ context.Context, _ reviewThreadRow) error { return e.err }
func (e *errInner) Delete(_ context.Context, _ string) error        { return e.err }
func (e *errInner) FindByKey(_ context.Context, _ string) (*reviewThreadRow, error) {
	return nil, e.err
}
func (e *errInner) FindAll(_ context.Context) ([]reviewThreadRow, error) { return nil, e.err }

// badDataInner returns a row with corrupt JSON data.
type badDataInner struct{}

func (b *badDataInner) Save(_ context.Context, _ reviewThreadRow) error { return nil }
func (b *badDataInner) Delete(_ context.Context, _ string) error        { return nil }
func (b *badDataInner) FindByKey(_ context.Context, _ string) (*reviewThreadRow, error) {
	return &reviewThreadRow{ID: "x", Data: []byte("not-json")}, nil
}

func (b *badDataInner) FindAll(_ context.Context) ([]reviewThreadRow, error) {
	return []reviewThreadRow{{ID: "x", Data: []byte("not-json")}}, nil
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
	th := domain.ReviewThread{
		ID:        "t1",
		WsID:      "w1",
		FilePath:  "a.go",
		Status:    domain.ReviewThreadStatusOpen,
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	require.NoError(t, st.Save(ctx, th))

	got, err := st.FindByKey(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "a.go", got.FilePath)

	all, err := st.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, st.Delete(ctx, "t1"))
	got, err = st.FindByKey(ctx, "t1")
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
	_, err := st.FindByKey(ctx, "t1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread storage: find")
}

func TestStorage_FindByKey_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindByKey(ctx, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread storage: unmarshal")
}

func TestStorage_FindAll_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread storage: find all")
}

func TestStorage_FindAll_BadData(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &badDataInner{}}
	_, err := st.FindAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread storage: unmarshal")
}

func TestStorage_Save_InnerError(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: &errInner{err: errors.New("db down")}}
	err := st.Save(ctx, domain.ReviewThread{ID: "t1"})
	require.Error(t, err)
}

// legacyThreadJSON is a read_review_threads row exactly as it was written before
// agent attribution existed — an agent message with no providerId and no chatId,
// which is what every row in every existing install holds. The blob is
// hand-written rather than produced by marshalling the current struct, because
// marshalling today's type can never reproduce yesterday's bytes.
const legacyThreadJSON = `{
	"id":"t1",
	"wsId":"w1",
	"filePath":"src/auth.go",
	"lineNumber":42,
	"side":"right",
	"status":"open",
	"messages":[
		{"id":"m1","author":"claude","isAgent":true,"body":"This leaks the token.","createdAt":"2026-06-19T10:00:00Z"},
		{"id":"m2","author":"","isAgent":false,"body":"fixed","createdAt":"2026-06-19T10:05:00Z"}
	],
	"createdAt":"2026-06-19T10:00:00Z"
}`

type legacyInner struct{}

func (legacyInner) Save(_ context.Context, _ reviewThreadRow) error { return nil }
func (legacyInner) Delete(_ context.Context, _ string) error        { return nil }

func (legacyInner) FindByKey(_ context.Context, _ string) (*reviewThreadRow, error) {
	return &reviewThreadRow{ID: "t1", Data: []byte(legacyThreadJSON)}, nil
}

func (legacyInner) FindAll(_ context.Context) ([]reviewThreadRow, error) {
	return []reviewThreadRow{{ID: "t1", Data: []byte(legacyThreadJSON)}}, nil
}

// TestStorage_PreAttributionRowLoadsCleanly is the no-migration guard. Review
// threads are persisted as opaque JSON blobs, so ProviderID and ChatID were
// added without any schema change — which is only safe if a row written before
// they existed still loads, with the whole thread intact and the two new fields
// simply empty. That is the common case forever, not an edge case.
func TestStorage_PreAttributionRowLoadsCleanly(t *testing.T) {
	ctx := context.Background()
	st := &storageStore{inner: legacyInner{}}

	got, err := st.FindByKey(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "src/auth.go", got.FilePath)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, "This leaks the token.", got.Messages[0].Body)
	assert.True(t, got.Messages[0].IsAgent)
	assert.Empty(t, got.Messages[0].ProviderID)
	assert.Empty(t, got.Messages[0].ChatID)
	assert.Empty(t, got.Messages[1].ProviderID)
	assert.Empty(t, got.Messages[1].ChatID)

	all, err := st.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Empty(t, all[0].Messages[0].ProviderID)
}

// TestStorage_AttributionSurvivesTheBlob is the write half: a thread saved with
// attribution reads back carrying it, through the same JSON column.
func TestStorage_AttributionSurvivesTheBlob(t *testing.T) {
	ctx, st := newStorage(t)
	require.NoError(t, st.Save(ctx, domain.ReviewThread{
		ID:     "t2",
		WsID:   "w1",
		Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID: "m1", Author: "claude", IsAgent: true,
			ProviderID: "claude", ChatID: "chat-7",
			Body: "finding", CreatedAt: time.Unix(1, 0).UTC(),
		}},
		CreatedAt: time.Unix(1, 0).UTC(),
	}))

	got, err := st.FindByKey(ctx, "t2")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, "claude", got.Messages[0].ProviderID)
	assert.Equal(t, "chat-7", got.Messages[0].ChatID)
}

func TestUnmarshalReviewThread_BadJSON(t *testing.T) {
	_, err := unmarshalReviewThread([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread storage: unmarshal")
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

	_, err = New(db, nil, nil, func(domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread store")
}
