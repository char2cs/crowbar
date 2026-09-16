package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newProjectStore(
	t *testing.T,
) (context.Context, store.Store[domain.Project, string]) {
	t.Helper()
	s, err := sqlite.New[domain.Project, string](":memory:")
	require.NoError(t, err)
	return context.Background(), s
}

func TestGormStore_SaveAndFindByKey(t *testing.T) {
	ctx, s := newProjectStore(t)
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p1", Name: "Alpha"}))
	got, err := s.FindByKey(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Alpha", got.Name)
}

func TestGormStore_FindByKey_NotFound_ReturnsNil(t *testing.T) {
	ctx, s := newProjectStore(t)
	got, err := s.FindByKey(ctx, "missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGormStore_FindAll(t *testing.T) {
	ctx, s := newProjectStore(t)
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p2"}))
	all, err := s.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestGormStore_Delete(t *testing.T) {
	ctx, s := newProjectStore(t)
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, s.Delete(ctx, "p1"))
	got, err := s.FindByKey(ctx, "p1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestNew_InvalidPath_ReturnsError(t *testing.T) {
	_, err := sqlite.New[domain.Project, string]("/nonexistent-dir-crowbar/x.db")
	assert.Error(t, err)
}

func TestGormStore_FindAll_Empty(t *testing.T) {
	ctx, s := newProjectStore(t)
	all, err := s.FindAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestGormStore_FindAll_ContextCancelled(t *testing.T) {
	_, s := newProjectStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.FindAll(ctx)
	assert.Error(t, err)
}

func TestGormStore_FindByKey_ContextCancelled(t *testing.T) {
	_, s := newProjectStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.FindByKey(ctx, "p1")
	assert.Error(t, err)
}

// TestOpenDB_ReadonlyDB_JournalModeError covers OpenDB's PRAGMA
// journal_mode=WAL error branch: gorm.Open succeeds against an existing
// file, but the PRAGMA fails because the file and its parent directory have
// had write permission stripped.
func TestOpenDB_ReadonlyDB_JournalModeError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.db")

	db, err := sqlite.OpenDB(path)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	require.NoError(t, os.Chmod(path, 0o444))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(path, 0o644)
	})

	_, err = sqlite.OpenDB(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal_mode")
}

// noPrimaryKey has no field tagged (or named) as a primary key, exercising
// the "no primary key field found" error path shared by primaryKeyColumn,
// NewFromDB and New.
type noPrimaryKey struct {
	Name string
}

func TestNew_NoPrimaryKeyField_ReturnsError(t *testing.T) {
	_, err := sqlite.New[noPrimaryKey, string](":memory:")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no primary key field found")
}

// badSchemaField cannot be parsed by GORM's schema parser (function-typed
// fields are unsupported), exercising NewFromDB's AutoMigrate error branch.
type badSchemaField struct {
	F func()
}

func TestNewFromDB_AutoMigrateError(t *testing.T) {
	db, err := sqlite.OpenDB(":memory:")
	require.NoError(t, err)

	_, err = sqlite.NewFromDB[badSchemaField, string](db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sqlite: migrate:")
}

// scopedFixture is a local GORM model exercising the WIDE ScopedStore surface —
// a repo-scoped FindWhere rather than a whole-table scan — independent of any
// real domain type, so this generic-adapter coverage does not ride on a
// particular aggregate's schema.
type scopedFixture struct {
	ID        string `gorm:"primaryKey" json:"id"`
	RepoID    string `json:"repoId"`
	ProjectID string `json:"projectId"`
	ParentID  string `json:"parentId,omitempty"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
}

func newScopedFixtureStore(
	t *testing.T,
) (context.Context, store.ScopedStore[scopedFixture, string]) {
	t.Helper()
	s, err := sqlite.New[scopedFixture, string](":memory:")
	require.NoError(t, err)
	return context.Background(), s
}

func TestGormStore_FindWhere_NarrowsToTheMatchingRows(t *testing.T) {
	ctx, s := newScopedFixtureStore(t)
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f1", ProjectID: "p1", RepoID: "r1", Name: "a"}))
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f2", ProjectID: "p1", RepoID: "r2", Name: "b"}))
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f3", ProjectID: "p2", RepoID: "r1", Name: "c"}))

	got, err := s.FindWhere(ctx, scopedFixture{ProjectID: "p1", RepoID: "r1"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "f1", got[0].ID)
}

// Zero fields are "don't care", not "must be empty" — the semantics the
// interface documents, and the reason parent filtering happens in memory.
func TestGormStore_FindWhere_IgnoresZeroFields(t *testing.T) {
	ctx, s := newScopedFixtureStore(t)
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f1", ProjectID: "p1", RepoID: "r1", ParentID: "w1"}))
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f2", ProjectID: "p1", RepoID: "r1"}))

	got, err := s.FindWhere(ctx, scopedFixture{ProjectID: "p1", RepoID: "r1"})
	require.NoError(t, err)
	assert.Len(t, got, 2, "an empty ParentID in the prototype filters nothing")
}

func TestGormStore_FindWhere_NoMatchReturnsEmpty(t *testing.T) {
	ctx, s := newScopedFixtureStore(t)
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f1", ProjectID: "p1", RepoID: "r1"}))

	got, err := s.FindWhere(ctx, scopedFixture{ProjectID: "p9", RepoID: "r9"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// `order` is a SQL keyword. GORM quotes it, but a silent failure here would show
// up as every sidebar row collapsing to index 0, so it is pinned explicitly.
func TestGormStore_OrderColumnRoundTrips(t *testing.T) {
	ctx, s := newScopedFixtureStore(t)
	require.NoError(t, s.Save(ctx, scopedFixture{ID: "f1", ProjectID: "p1", RepoID: "r1", Order: 7}))

	got, err := s.FindByKey(ctx, "f1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 7, got.Order)
}

func TestGormStore_FindWhere_ClosedDB_ReturnsError(t *testing.T) {
	db, err := sqlite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := sqlite.NewFromDB[scopedFixture, string](db)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = s.FindWhere(context.Background(), scopedFixture{ProjectID: "p1"})
	assert.Error(t, err)
}
