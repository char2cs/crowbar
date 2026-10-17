package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func runMigrate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"migrate"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestMigrateCmd_PlanPrintsAndChangesNothingOnDisk(t *testing.T) {
	h := testutil.FixtureHome(t)
	t.Setenv(home.EnvVar, h.Root())
	testutil.WriteFile(t, h, "state/upgrades/chat-types", "")
	testutil.WriteFile(t, h, "state/view.db", "not a database: the plan must not open it")
	before := testutil.TreeSnapshot(t, h)

	out, err := runMigrate(t, "--plan")

	require.NoError(t, err)
	assert.Contains(t, out, "home      "+h.Root())
	assert.Contains(t, out, "5 pending steps, in execution order")
	assert.Contains(t, out, "#2 workspace-paths-table   pre-open   layout 0")
	assert.Contains(t, out, "state/view.db: drop table workspace_paths")
	assert.Contains(t, out, "#3 chat-types   post-open   layout 0")
	assert.Contains(t, out, "delete  state/upgrades/chat-types")
	assert.Contains(t, out, "state backup  required")
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
	assert.NoDirExists(t, h.MigrateDir())
	assert.NoFileExists(t, h.LayoutFile())
}

func TestMigrateCmd_PlanOfAnUnopenedHomeCreatesNothing(t *testing.T) {
	h := testutil.FixtureHome(t)
	t.Setenv(home.EnvVar, h.Root())

	out, err := runMigrate(t, "--plan")

	require.NoError(t, err)
	assert.Contains(t, out, "layout    0 (legacy)")
	assert.Contains(t, out, "state backup  not needed")
	assert.Empty(t, testutil.TreeSnapshot(t, h))
	assert.NoDirExists(t, h.State())
}

func TestMigrateCmd_RequiresPlan(t *testing.T) {
	h := testutil.FixtureHome(t)
	t.Setenv(home.EnvVar, h.Root())

	out, err := runMigrate(t)

	require.ErrorContains(t, err, "use --plan")
	assert.Empty(t, out)
	assert.Empty(t, testutil.TreeSnapshot(t, h))
}

func TestMigrateCmd_NewerLayoutExitsNonZero(t *testing.T) {
	h := testutil.FixtureHome(t)
	t.Setenv(home.EnvVar, h.Root())
	testutil.WriteFile(t, h, "LAYOUT", "9\n")

	out, err := runMigrate(t, "--plan")

	require.ErrorContains(t, err, "layout 9")
	assert.Empty(t, out)
}

func TestMigrateCmd_APreflightHaltExitsNonZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink on Windows needs privileges the runner may lack")
	}
	h := testutil.FixtureHome(t)
	t.Setenv(home.EnvVar, h.Root())
	testutil.WriteFile(t, h, "state/view.db", "rows")
	require.NoError(t, os.Symlink(filepath.Join(h.State(), "view.db"), filepath.Join(h.State(), "alias.db")))
	before := testutil.TreeSnapshot(t, h)

	out, err := runMigrate(t, "--plan")

	require.ErrorContains(t, err, "unsupported symlink", "the boot would halt on it, so the preview does")
	assert.Empty(t, out)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}
