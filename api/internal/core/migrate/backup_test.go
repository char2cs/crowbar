package migrate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

type treeEntry struct {
	Dir     bool
	Content string
	Mode    fs.FileMode
}

func plentyOfDisk(
	_ string,
) (uint64, error) {
	return 1 << 40, nil
}

func diskOf(
	free uint64,
) FreeBytesFunc {
	return func(string) (uint64, error) {
		return free, nil
	}
}

func readTree(
	tb testing.TB,
	root string,
) map[string]treeEntry {
	tb.Helper()
	out := map[string]treeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == root {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out[filepath.ToSlash(rel)] = treeEntry{Dir: true}
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = treeEntry{Content: string(raw), Mode: info.Mode().Perm()}
		return nil
	})
	require.NoError(tb, err)
	return out
}

func assertNoBackupFor(
	tb testing.TB,
	h home.Home,
	runID string,
) {
	tb.Helper()
	backups := filepath.Join(h.MigrateDir(), "backups")
	assert.NoDirExists(tb, filepath.Join(backups, runID+".partial"))
	assert.NoDirExists(tb, filepath.Join(backups, runID))
}

func TestBackupState_CopiesTheTreeByteForByte(t *testing.T) {
	h := testutil.FixtureHome(t)
	blob := make([]byte, 256)
	for i := range blob {
		blob[i] = byte(i)
	}
	testutil.WriteFile(t, h, "state/events/a.db", "events database")
	testutil.WriteFile(t, h, "state/events/a.db-wal", "write-ahead log")
	testutil.WriteFile(t, h, "state/content/aa/bb/blob", string(blob))
	testutil.WriteFile(t, h, "state/view.db", "view")
	testutil.WriteFile(t, h, "state/events/.lock", "user data, not the home lock")
	require.NoError(t, os.MkdirAll(filepath.Join(h.State(), "store"), 0o700))
	require.NoError(t, os.Chmod(filepath.Join(h.State(), "view.db"), 0o664))
	lock, err := lockHome(h)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lock.Close()) })
	var probed string

	got, err := backupState(h, "run-1", func(path string) (uint64, error) {
		probed = path
		return plentyOfDisk(path)
	})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(h.MigrateDir(), "backups", "run-1", "state"), got.Path)
	assert.Equal(t, int64(len("events database")+len("write-ahead log")+len(blob)+len("view")+len("user data, not the home lock")), got.Bytes)
	assert.Equal(t, h.Root(), probed)
	want := readTree(t, h.State())
	delete(want, ".lock")
	assert.Equal(t, want, readTree(t, got.Path))
	assert.Contains(t, want, "events/.lock")
	assert.Contains(t, want, "store")
	assert.NoDirExists(t, filepath.Join(h.MigrateDir(), "backups", "run-1.partial"))
}

func TestBackupState_EmptyOrMissingStateMakesNoBackup(t *testing.T) {
	t.Run("no state directory", func(t *testing.T) {
		h := testutil.FixtureHome(t)

		got, err := backupState(h, "run-1", plentyOfDisk)

		require.NoError(t, err)
		assert.Equal(t, backupResult{}, got)
		assert.NoDirExists(t, h.MigrateDir())
	})

	t.Run("empty state directory", func(t *testing.T) {
		h := testutil.FixtureHome(t)
		require.NoError(t, os.MkdirAll(h.State(), 0o700))

		got, err := backupState(h, "run-1", plentyOfDisk)

		require.NoError(t, err)
		assert.Equal(t, backupResult{}, got)
		assert.NoDirExists(t, h.MigrateDir())
	})

	t.Run("only the home lock and empty directories", func(t *testing.T) {
		h := testutil.FixtureHome(t)
		require.NoError(t, os.MkdirAll(filepath.Join(h.State(), "events"), 0o700))
		lock, err := lockHome(h)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, lock.Close()) })

		got, err := backupState(h, "run-1", plentyOfDisk)

		require.NoError(t, err)
		assert.Equal(t, backupResult{}, got)
		assert.NoDirExists(t, h.MigrateDir())
	})

	t.Run("an empty regular file is still a file", func(t *testing.T) {
		h := testutil.FixtureHome(t)
		testutil.WriteFile(t, h, "state/empty.db", "")

		got, err := backupState(h, "run-1", plentyOfDisk)

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(h.MigrateDir(), "backups", "run-1", "state"), got.Path)
		assert.Equal(t, readTree(t, h.State()), readTree(t, got.Path))
	})
}

func TestBackupState_InsufficientDiskLeavesNothingBehind(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/events/a.db", string(make([]byte, 1000)))
	testutil.WriteFile(t, h, "state/view.db", "v")
	before := testutil.TreeSnapshot(t, h)

	_, err := backupState(h, "run-1", diskOf(1001+backupHeadroom-1))

	require.ErrorIs(t, err, ErrInsufficientDisk)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
	assert.NoDirExists(t, h.MigrateDir())
}

func TestBackupState_AcceptsExactlyTheRequiredHeadroom(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/events/a.db", string(make([]byte, 1000)))
	testutil.WriteFile(t, h, "state/view.db", "v")

	got, err := backupState(h, "run-1", diskOf(1001+backupHeadroom))

	require.NoError(t, err)
	assert.Equal(t, int64(1001), got.Bytes)
}

func TestBackupState_AFailingProbeLeavesNothingBehind(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/view.db", "v")
	probeFailed := errors.New("statfs unavailable")

	_, err := backupState(h, "run-1", func(string) (uint64, error) { return 0, probeFailed })

	require.ErrorIs(t, err, probeFailed)
	assert.NoDirExists(t, h.MigrateDir())
}

func TestFreeBytes_ReportsTheVolumeOfAnExistingPath(t *testing.T) {
	got, err := freeBytes(t.TempDir())

	require.NoError(t, err)
	assert.Positive(t, got)
}

func TestBackupState_RejectsAnUnsupportedEntry(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/a.db", "a")
	testutil.WriteFile(t, h, "state/target", "t")
	link := filepath.Join(h.State(), "z-link")
	if err := os.Symlink(filepath.Join(h.State(), "target"), link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("creating a symlink on Windows needs privileges the runner may lack: %v", err)
		}
		require.NoError(t, err)
	}

	_, err := backupState(h, "run-1", plentyOfDisk)

	require.ErrorContains(t, err, link)
	assertNoBackupFor(t, h, "run-1")
}

func TestBackupState_AFileThatCannotBeReadLeavesNothingBehind(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/a.db", "a")
	testutil.WriteFile(t, h, "state/z.db", "z")
	require.NoError(t, os.Chmod(filepath.Join(h.State(), "z.db"), 0o000))

	_, err := backupState(h, "run-1", plentyOfDisk)

	require.ErrorIs(t, err, fs.ErrPermission)
	assertNoBackupFor(t, h, "run-1")
}

func TestBackupState_RejectsARunIDThatIsNotOneSegment(t *testing.T) {
	for _, runID := range []string{"", ".", "..", "a/b", `a\b`, "../escape", "/abs", "run-1.partial"} {
		t.Run(runID, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			testutil.WriteFile(t, h, "state/a.db", "a")
			before := testutil.TreeSnapshot(t, h)

			_, err := backupState(h, runID, plentyOfDisk)

			require.Error(t, err)
			assert.Equal(t, before, testutil.TreeSnapshot(t, h))
			assert.NoDirExists(t, h.MigrateDir())
		})
	}
}

func TestCleanPartialBackups_RemovesOnlyPartials(t *testing.T) {
	t.Run("keeps finished backups", func(t *testing.T) {
		h := testutil.FixtureHome(t)
		testutil.WriteFile(t, h, "migrate/backups/crashed.partial/state/events/a.db", "half")
		testutil.WriteFile(t, h, "migrate/backups/done/state/view.db", "whole")

		require.NoError(t, cleanPartialBackups(h))

		assert.NoDirExists(t, filepath.Join(h.MigrateDir(), "backups", "crashed.partial"))
		assert.FileExists(t, filepath.Join(h.MigrateDir(), "backups", "done", "state", "view.db"))
	})

	t.Run("no migrate directory is not an error and creates nothing", func(t *testing.T) {
		h := testutil.FixtureHome(t)

		require.NoError(t, cleanPartialBackups(h))

		assert.NoDirExists(t, h.MigrateDir())
	})

	t.Run("an empty backups directory is left alone", func(t *testing.T) {
		h := testutil.FixtureHome(t)
		require.NoError(t, os.MkdirAll(filepath.Join(h.MigrateDir(), "backups"), 0o700))

		require.NoError(t, cleanPartialBackups(h))

		assert.DirExists(t, filepath.Join(h.MigrateDir(), "backups"))
	})
}

func TestTreeSize_SumsFilesAndSkipsTheLock(t *testing.T) {
	dir := t.TempDir()
	for rel, content := range map[string]string{
		"a":           "abc",
		"sub/b":       "12345",
		".lock":       string(make([]byte, 100)),
		"sub/.lock":   "1234567",
		"sub/deep/.x": "",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "empty"), 0o700))

	got, err := treeSize(dir)

	require.NoError(t, err)
	assert.Equal(t, int64(3+5+7), got)

	missing, err := treeSize(filepath.Join(dir, "missing"))

	require.NoError(t, err)
	assert.Zero(t, missing)
}
