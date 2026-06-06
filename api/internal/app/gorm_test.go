package app

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	glebarez "github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// failOnNthExecPool wraps a *sql.DB and injects an error on ExecContext calls
// after the first failAt successful calls. QueryRowContext always passes through
// because *sql.Row carries no error until Scan; ExecContext is what the SQLite
// AutoMigrate CREATE TABLE statement goes through.
type failOnNthExecPool struct {
	inner  *sql.DB
	mu     sync.Mutex
	count  int
	failAt int
}

func (p *failOnNthExecPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.inner.PrepareContext(ctx, query)
}

func (p *failOnNthExecPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	p.mu.Lock()
	p.count++
	n := p.count
	p.mu.Unlock()
	if n > p.failAt {
		return nil, errors.New("injected exec failure")
	}
	return p.inner.ExecContext(ctx, query, args...)
}

func (p *failOnNthExecPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.inner.QueryContext(ctx, query, args...)
}

func (p *failOnNthExecPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.inner.QueryRowContext(ctx, query, args...)
}

// GetDBConn satisfies gorm's GetDBConnector interface used internally by GORM.
func (p *failOnNthExecPool) GetDBConn() (*sql.DB, error) { return p.inner, nil }

// newFailAfterExecDB opens an in-memory SQLite GORM DB whose ExecContext will
// succeed for the first failAt calls and return an error for every subsequent
// call. Both db.ConnPool and db.Statement.ConnPool are replaced so that all
// operations go through the wrapper.
func newFailAfterExecDB(t *testing.T, failAt int) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(glebarez.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	pool := &failOnNthExecPool{inner: sqlDB, failAt: failAt}
	db.ConnPool = pool
	db.Statement.ConnPool = pool
	return db
}

func TestNewGORMStores_ProjectRoundTrips(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	stores, err := newGORMStores(db)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, stores.Projects.Save(ctx, domain.Project{ID: "p1", Name: "Alpha"}))
	got, err := stores.Projects.FindByKey(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Alpha", got.Name)
}

func TestNewGORMStores_ClosedDB_ReturnsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = newGORMStores(db)
	assert.Error(t, err)
}

// TestNewGORMStores_RepositoryStoreError triggers the "repository store" error
// branch by letting only the first ExecContext (Project CREATE TABLE) succeed.
// SQLite AutoMigrate issues exactly one ExecContext per table, so failAt=1
// causes repositories CREATE TABLE to fail.
func TestNewGORMStores_RepositoryStoreError(t *testing.T) {
	db := newFailAfterExecDB(t, 1)
	_, err := newGORMStores(db)
	require.Error(t, err)
	assert.ErrorContains(t, err, "app: repository store:")
}

// TestNewGORMStores_TerminalProfileStoreError triggers the "terminal profile
// store" error branch by allowing the first two ExecContext calls (Project and
// Repository CREATE TABLE) to succeed before injecting a failure.
func TestNewGORMStores_TerminalProfileStoreError(t *testing.T) {
	db := newFailAfterExecDB(t, 2)
	_, err := newGORMStores(db)
	require.Error(t, err)
	assert.ErrorContains(t, err, "app: terminal profile store:")
}
