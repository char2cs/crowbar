package app

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	glebarez "github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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

func (p *failOnNthExecPool) GetDBConn() (*sql.DB, error) { return p.inner, nil }

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

func TestNewGORMStores_RepositoryStoreError(t *testing.T) {
	db := newFailAfterExecDB(t, 1)
	_, err := newGORMStores(db)
	require.Error(t, err)
	assert.ErrorContains(t, err, "app: repository store:")
}

func TestNewGORMStores_TerminalProfileStoreError(t *testing.T) {
	db := newFailAfterExecDB(t, 2)
	_, err := newGORMStores(db)
	require.Error(t, err)
	assert.ErrorContains(t, err, "app: terminal profile store:")
}

func TestNewGORMStores_TerminalSessionStoreError(t *testing.T) {
	db := newFailAfterExecDB(t, 3)
	_, err := newGORMStores(db)
	require.Error(t, err)
	assert.ErrorContains(t, err, "app: terminal session store:")
}

func TestGORMStores_Clients_SaveFindRevoke(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	stores, err := newGORMStores(db)
	require.NoError(t, err)

	ctx := context.Background()
	c := domain.Client{
		ID:        "client-1",
		TokenHash: "deadbeef",
		Name:      "test-device",
		PairedAt:  time.Now(),
		LastSeen:  time.Now(),
	}
	require.NoError(t, stores.Clients.Save(ctx, c))

	found, err := stores.Clients.FindWhere(ctx, domain.Client{TokenHash: "deadbeef"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "client-1", found[0].ID)

	now := time.Now()
	found[0].RevokedAt = &now
	require.NoError(t, stores.Clients.Save(ctx, found[0]))

	revoked, err := stores.Clients.FindByKey(ctx, "client-1")
	require.NoError(t, err)
	require.NotNil(t, revoked)
	require.NotNil(t, revoked.RevokedAt)
}

func TestNewGORMStores_TerminalSessionRoundTrip(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	stores, err := newGORMStores(db)
	require.NoError(t, err)

	ctx := context.Background()
	sess := domain.TerminalSession{
		SessionID: "sess-1",
		ChatID:    "chat-1",
		ProjectID: "proj-1",
		RepoID:    "repo-1",
		State:     "active",
	}
	require.NoError(t, stores.TerminalSessions.Save(ctx, sess))

	got, err := stores.TerminalSessions.FindByKey(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "chat-1", got.ChatID)
	assert.Equal(t, "proj-1", got.ProjectID)
	assert.Equal(t, "active", got.State)

	require.NoError(t, stores.TerminalSessions.Delete(ctx, "sess-1"))
	gone, err := stores.TerminalSessions.FindByKey(ctx, "sess-1")
	require.NoError(t, err)
	assert.Nil(t, gone)
}
