package app_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	glebarez "github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// containerExecFailPool is the same counting ConnPool used in gorm_test.go but
// scoped to the app_test package so that container_test.go can construct
// adapter.Container instances with a deliberately broken DB.
type containerExecFailPool struct {
	inner  *sql.DB
	mu     sync.Mutex
	count  int
	failAt int
}

func (p *containerExecFailPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.inner.PrepareContext(ctx, query)
}

func (p *containerExecFailPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	p.mu.Lock()
	p.count++
	n := p.count
	p.mu.Unlock()
	if n > p.failAt {
		return nil, errors.New("injected exec failure")
	}
	return p.inner.ExecContext(ctx, query, args...)
}

func (p *containerExecFailPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.inner.QueryContext(ctx, query, args...)
}

func (p *containerExecFailPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.inner.QueryRowContext(ctx, query, args...)
}

func (p *containerExecFailPool) GetDBConn() (*sql.DB, error) { return p.inner, nil }

// newContainerFailDB builds a GORM DB whose ExecContext succeeds for the first
// failAt calls then injects an error.
func newContainerFailDB(t *testing.T, failAt int) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(glebarez.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	pool := &containerExecFailPool{inner: sqlDB, failAt: failAt}
	db.ConnPool = pool
	db.Statement.ConnPool = pool
	return db
}

func newInMemES(t *testing.T) asynxModels.Store {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	return es
}

func newEng(t *testing.T) *engine.Container {
	t.Helper()
	eng, err := engine.New(context.Background())
	require.NoError(t, err)
	return eng
}

func TestApp_New_BootsFullLayer(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)
	assert.NotNil(t, c.Hub)
	assert.NotNil(t, c.Repositories)
	assert.NotNil(t, c.GORM)
	assert.NotNil(t, c.Repositories.Workspace)
	assert.NotNil(t, c.Usecases)
	assert.NotNil(t, c.Usecases.Workspace)
}

func TestApp_New_UsecasesWorkspaceListEndToEnd(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, newEng(t), adapters)
	require.NoError(t, err)

	_, err = c.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)

	rows, err := c.Usecases.Workspace.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
}

func TestApp_New_NilWorkspaceES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.WorkspaceES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_NilChatES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.ChatES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_NilReviewThreadES_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	adapters.ReviewThreadES = nil

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

func TestApp_New_ClosedDB_ReturnsError(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	sqlDB, err := adapters.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = app.New(context.Background(), newEng(t), adapters)
	assert.Error(t, err)
}

// TestApp_New_RepositoriesNewError triggers the "app: repositories:" error branch
// in app.New by letting newGORMStores succeed (3 ExecContext calls for
// Project/Repository/TerminalProfile CREATE TABLE) but failing on the 4th call,
// which is the workspace read-model AutoMigrate inside repositories.New.
func TestApp_New_RepositoriesNewError(t *testing.T) {
	db := newContainerFailDB(t, 3)
	adapters := &adapter.Container{
		WorkspaceES:    newInMemES(t),
		ChatES:         newInMemES(t),
		ReviewThreadES: newInMemES(t),
		DB:             db,
	}

	_, err := app.New(context.Background(), newEng(t), adapters)
	require.Error(t, err)
	assert.ErrorContains(t, err, "app: repositories:")
}
