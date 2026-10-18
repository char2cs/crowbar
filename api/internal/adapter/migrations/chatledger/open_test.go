package chatledger

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	glebarez "github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writable opens path the way the old daemon did: read-write, in WAL mode.
func writable(t *testing.T, path string, ddl ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open(glebarez.DriverName, path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(t.Context(), "PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	for _, stmt := range ddl {
		_, err = db.ExecContext(t.Context(), stmt)
		require.NoError(t, err)
	}
	return db
}

func dirListing(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := map[string][]byte{}
	for _, e := range entries {
		blob, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		out[e.Name()] = blob
	}
	return out
}

func TestOpenLegacy_ReadsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, RunnerDB)
	w := writable(t, path, PlacementsDDL)
	_, err := w.ExecContext(t.Context(), "INSERT INTO agent_chat_placements (chat_id, provider_id) VALUES ('c1', 'claude')")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	before := dirListing(t, dir)
	require.NotContains(t, before, RunnerDB+"-wal", "the writer checkpointed on close")

	db, ok, err := openLegacy(path, false)
	require.NoError(t, err)
	require.True(t, ok)
	var provider string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT provider_id FROM agent_chat_placements").Scan(&provider))
	_, writeErr := db.ExecContext(t.Context(), "DELETE FROM agent_chat_placements")
	require.NoError(t, db.Close())

	assert.Equal(t, "claude", provider)
	require.Error(t, writeErr, "a legacy database is opened read-only")
	assert.Equal(t, before, dirListing(t, dir), "reading created, removed or changed no file")
}

func TestOpenLegacy_ReadsRowsStillInTheWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), RunnerDB)
	w := writable(t, path, PlacementsDDL)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	_, err := w.ExecContext(t.Context(), "PRAGMA wal_autocheckpoint=0")
	require.NoError(t, err)
	_, err = w.ExecContext(t.Context(), "INSERT INTO agent_chat_placements (chat_id, provider_id) VALUES ('c1', 'codex')")
	require.NoError(t, err)
	wal, err := os.Stat(path + "-wal")
	require.NoError(t, err)
	require.Positive(t, wal.Size(), "the row must still be in the WAL for this test to mean anything")

	db, ok, err := openLegacy(path, false)
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var provider string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT provider_id FROM agent_chat_placements").Scan(&provider))
	assert.Equal(t, "codex", provider)
}

// A database this daemon still writes can checkpoint its WAL to nothing just
// before the read opens it; opened immutable, the read misses every later commit.
func TestOpenLegacy_ADatabaseStillWrittenSeesLaterCommits(t *testing.T) {
	path := filepath.Join(t.TempDir(), ActivityDB)
	w := writable(t, path, InterruptionsDDL)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	_, err := w.ExecContext(t.Context(), "PRAGMA wal_checkpoint(TRUNCATE)")
	require.NoError(t, err)
	wal, err := os.Stat(path + "-wal")
	require.NoError(t, err)
	require.Zero(t, wal.Size(), "the WAL must be empty for this test to mean anything")

	db, ok, err := openLegacy(path, true)
	require.NoError(t, err)
	require.True(t, ok)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	count := func() (n int) {
		require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM agent_interruptions").Scan(&n))
		return n
	}
	require.Zero(t, count())
	_, err = w.ExecContext(t.Context(), "INSERT INTO agent_interruptions (key, chat_id, kind, detail) VALUES ('k1', 'c1', ?, 'codex')",
		providerSwitched)
	require.NoError(t, err)

	assert.Equal(t, 1, count())
}

func TestOpenLegacy_AMissingFileIsEmptyAndStaysMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), RunnerDB)

	db, ok, err := openLegacy(path, false)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, db)
	assert.NoFileExists(t, path)
}
