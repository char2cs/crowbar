package migrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestSameInBackup_ComparesTheLiveFilesWithTheNewestBackup(t *testing.T) {
	b := newBench(t, rewriting(1, "a"))
	testutil.WriteFile(t, b.home, "state/kept.db", "unchanged")
	testutil.WriteFile(t, b.home, "state/moved.db", "before")
	boot(t, b, tracing(new([]int), 1))
	testutil.WriteFile(t, b.home, "state/moved.db", "after")
	testutil.WriteFile(t, b.home, "state/new.db", "never backed up")
	backup, err := History{home: b.home}.Backup()
	require.NoError(t, err)

	same, err := SameInBackup(backup, b.home.Root(), []string{"state/kept.db", "state/moved.db", "state/new.db"})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"state/kept.db": true, "state/moved.db": false}, same)
}

func TestSameInBackup_ReadsADirectoryBackupForStateAlone(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/a.db", "same")
	testutil.WriteFile(t, h, "chats/c/prompt-requests/p.json", "journal")
	testutil.WriteFile(t, h, "migrate/backups/20261016T010203.000000000Z/state/a.db", "same")
	backup := filepath.Join(h.MigrateDir(), backupsDirName, "20261016T010203.000000000Z")

	same, err := SameInBackup(backup, h.Root(), []string{"state/a.db", "chats/c/prompt-requests/p.json"})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"state/a.db": true}, same)
}

func TestVerifyArchive_RefusesAnArchiveThatDoesNotReadBackAsWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.tar.gz")
	a, err := writeArchive(path, func(a *archiver) error { return a.addBytes("LAYOUT", []byte("1\n")) })
	require.NoError(t, err)
	require.NoError(t, verifyArchive(path, a.sums))

	other := digests{"LAYOUT": a.sums["LAYOUT"], "state/view.db": a.sums["LAYOUT"]}

	require.ErrorContains(t, verifyArchive(path, other), "does not read back as written")
}

func TestBackupState_FollowsASymlinkedStateRoot(t *testing.T) {
	h := testutil.FixtureHome(t)
	elsewhere := filepath.Join(filepath.Dir(h.Root()), "other-disk-state")
	for rel, content := range map[string]string{
		"events/a.db":      "events database",
		"content/aa/blob":  "blob bytes",
		"view.db":          "view",
		"events/.lock":     "user data, not the home lock",
		"store/.keep-dirs": "",
	} {
		path := filepath.Join(elsewhere, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	symlinkOrSkip(t, elsewhere, h.State())
	startedRun(t, h)
	lock, err := lockHome(h)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lock.Close()) })
	var probed string

	stats, statsErr := measureState(h.State())
	got, err := backupState(h, "run-1", 0, nil, func(path string) (uint64, error) {
		probed = path
		return plentyOfDisk(path)
	})

	require.NoError(t, statsErr)
	require.NoError(t, err)
	want := readTree(t, elsewhere)
	delete(want, ".lock")
	assert.Equal(t, archived(want, "state"), only(readBackup(t, got.Path), "state"))
	assert.Equal(t, filepath.Join(h.MigrateDir(), "backups", "run-1.tar.gz"), got.Path)
	assert.Equal(t, h.Root(), probed)
	assert.Equal(t, got.Bytes, stats.bytes)
	assert.Equal(t, int64(len("events database")+len("blob bytes")+len("view")+len("user data, not the home lock")), stats.bytes)
}

func TestBackupState_ADanglingStateRootIsAnError(t *testing.T) {
	h := testutil.FixtureHome(t)
	symlinkOrSkip(t, filepath.Join(filepath.Dir(h.Root()), "gone"), h.State())

	_, err := backupState(h, "run-1", 0, nil, plentyOfDisk)

	require.Error(t, err)
	assert.NoDirExists(t, h.MigrateDir())
}

func TestBackupState_AFileThatCannotBeReadLeavesNothingBehind(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/a.db", "a")
	testutil.WriteFile(t, h, "state/z.db", "z")
	require.NoError(t, os.Chmod(filepath.Join(h.State(), "z.db"), 0o000))

	_, err := backupState(h, "run-1", 0, nil, plentyOfDisk)

	require.ErrorIs(t, err, fs.ErrPermission)
	assertNoBackupFor(t, h, "run-1")
}
