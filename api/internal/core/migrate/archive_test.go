package migrate

import (
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
