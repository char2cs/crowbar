package migrations

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestRemoveMarker_TakesTheMarkerAndTheDirItEmptied(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/upgrades/chat-types", "")

	require.NoError(t, removeMarker(h, "chat-types"))

	assert.NoDirExists(t, filepath.Join(h.State(), "upgrades"))
	assert.DirExists(t, h.State())
}

func TestRemoveMarker_LeavesTheOtherMarkers(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/upgrades/chat-types", "")
	testutil.WriteFile(t, h, "state/upgrades/hook-deliveries", "")

	require.NoError(t, removeMarker(h, "chat-types"))

	assert.Equal(t, []string{"state/upgrades/hook-deliveries 0"}, testutil.TreeSnapshot(t, h))
}

func TestRemoveMarker_AbsentIsNothingToDo(t *testing.T) {
	h := testutil.FixtureHome(t)

	require.NoError(t, removeMarker(h, "chat-types"))

	present, err := markerPresent(h, "chat-types")
	require.NoError(t, err)
	assert.False(t, present)
	assert.Empty(t, testutil.TreeSnapshot(t, h))
}

// An older daemon finished the step and left its marker: the step is recorded
// as adopted, the marker goes, and the step's own work never runs.
func TestAdopting_AMarkedStepIsAdoptedNotRun(t *testing.T) {
	h := testutil.FixtureHome(t)
	execViewDB(t, h, "CREATE TABLE workspace_paths (id TEXT)")
	testutil.WriteFile(t, h, "state/upgrades/workspace-paths-table", "")

	finish(t, boot(t, h))

	assert.True(t, viewDBHasTable(t, h, "workspace_paths"), "the old daemon's word is taken")
	assert.NoDirExists(t, filepath.Join(h.State(), "upgrades"))
	assert.Equal(t, []migrate.Counts{{"adopted": 1}}, migratetest.Applied(t, h)[StepWorkspacePathsTable])
}

// The adoption is recorded before the marker is deleted, so an attempt that
// stops in between is still an adoption on the next boot, never a re-run.
func TestAdopting_AnAdoptionOutlivesItsMarker(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	h := testutil.FixtureHome(t)
	execViewDB(t, h, "CREATE TABLE workspace_paths (id TEXT)")
	testutil.WriteFile(t, h, "state/upgrades/workspace-paths-table", "")
	upgrades := filepath.Join(h.State(), "upgrades")
	require.NoError(t, os.Chmod(upgrades, 0o500))
	t.Cleanup(func() {
		if err := os.Chmod(upgrades, 0o700); err != nil {
			require.ErrorIs(t, err, fs.ErrNotExist)
		}
	})

	sess, err := Begin(t.Context(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	var halt *migrate.Halt
	require.ErrorAs(t, sess.RunPreOpen(t.Context(), PreOpenImpls()), &halt)
	require.NoError(t, sess.Close())
	require.NoError(t, os.Chmod(upgrades, 0o700))
	require.NoError(t, os.Remove(filepath.Join(upgrades, "workspace-paths-table")))

	finish(t, boot(t, h))

	assert.True(t, viewDBHasTable(t, h, "workspace_paths"))
	assert.Equal(t, []migrate.Counts{{"adopted": 1}}, migratetest.Applied(t, h)[StepWorkspacePathsTable])
}
