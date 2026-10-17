package migrations

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestPreOpen_DropsTheRetiredTable(t *testing.T) {
	h := testutil.FixtureHome(t)
	execViewDB(t, h, "CREATE TABLE workspace_paths (id TEXT PRIMARY KEY, path TEXT)")

	finish(t, boot(t, h))

	assert.False(t, viewDBHasTable(t, h, "workspace_paths"))
	before := testutil.TreeSnapshot(t, h)
	finish(t, boot(t, h))
	assert.Equal(t, before, testutil.TreeSnapshot(t, h), "a second boot has nothing to do")
	assert.Len(t, migratetest.Applied(t, h)[StepWorkspacePathsTable], 1)
}

func TestPreOpen_NeverCreatesAViewDB(t *testing.T) {
	h := testutil.FixtureHome(t)

	finish(t, boot(t, h))

	assert.NoFileExists(t, viewDBPath(h))
}

func TestPreOpen_CorruptViewDBHaltsNamingTheStep(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/view.db", "this is not a sqlite database, not even a little")

	sess, err := Begin(context.Background(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })
	err = sess.RunPreOpen(context.Background(), PreOpenImpls())

	var halt *migrate.Halt
	require.ErrorAs(t, err, &halt)
	require.NotNil(t, halt.Step)
	assert.Equal(t, "workspace-paths-table", halt.Step.Name)
	assert.Same(t, halt, sess.Halt(), "the session stays live, holding the home")
}

func TestDropRetiredTable_DropsItAndToleratesItsAbsence(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE workspace_paths (id TEXT)").Error)

	require.NoError(t, dropRetiredTable(context.Background(), db, "workspace_paths"))
	require.NoError(t, dropRetiredTable(context.Background(), db, "workspace_paths"))
	assert.False(t, db.Migrator().HasTable("workspace_paths"))
}

// boot is a daemon's pre-open half on h; the session is closed with the test.
func boot(
	t *testing.T,
	h home.Home,
) *migrate.Session {
	t.Helper()
	sess, err := Begin(context.Background(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })
	require.NoError(t, sess.RunPreOpen(context.Background(), PreOpenImpls()))
	return sess
}

// finish completes the run with post-open steps that do nothing and closes
// the session, as a daemon that opened its stores would.
func finish(
	t *testing.T,
	sess *migrate.Session,
) {
	t.Helper()
	noop := migrate.Impl{Apply: func(context.Context, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error) {
		return nil, nil
	}}
	impls := migrate.Impls{StepProjectHomes: noop, StepChatTypes: noop, StepChatProviders: noop, StepHookDeliveries: noop}
	require.NoError(t, sess.RunPostOpen(context.Background(), impls))
	require.NoError(t, sess.Close())
}

func execViewDB(
	t *testing.T,
	h home.Home,
	sql string,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(h.State(), 0o700))
	db, err := storesqlite.OpenDB(viewDBPath(h))
	require.NoError(t, err)
	require.NoError(t, db.Exec(sql).Error)
	require.NoError(t, closeDB(db))
}

func viewDBHasTable(
	t *testing.T,
	h home.Home,
	table string,
) bool {
	t.Helper()
	db, err := storesqlite.OpenDB(viewDBPath(h))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeDB(db)) })
	return db.Migrator().HasTable(table)
}
