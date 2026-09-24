package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

// synchronousNormal is PRAGMA synchronous's numeric answer for NORMAL.
const synchronousNormal = 1

// AssertSynchronousNormal checks every one of n pooled connections, held at
// once so the pool cannot hand the same connection back twice: synchronous is
// per-connection state, so a setting that reached only one would pass a
// single-query check and still fsync on every other connection.
func assertSynchronousNormal(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	ctx := context.Background()
	for i := range n {
		conn, err := sqlDB.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		var mode int
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&mode))
		assert.Equal(t, synchronousNormal, mode, "connection %d must run synchronous=NORMAL", i)
	}
}

func TestOpenReadPoolDB_EveryConnectionIsSynchronousNormal(t *testing.T) {
	db, err := sqlite.OpenReadPoolDB(filepath.Join(t.TempDir(), "pool.db"))
	require.NoError(t, err)
	assertSynchronousNormal(t, db, 4)
}

func TestOpenDB_IsSynchronousNormal(t *testing.T) {
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "single.db"))
	require.NoError(t, err)
	assertSynchronousNormal(t, db, 1)
}

func TestDSN_AppendsToAnExistingQuery(t *testing.T) {
	assert.Equal(t, "a.db?_pragma=synchronous(NORMAL)", sqlite.DSN("a.db"))
	assert.Equal(t, "a.db?mode=ro&_pragma=synchronous(NORMAL)", sqlite.DSN("a.db?mode=ro"))
}
