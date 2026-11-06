package migrate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func rewriting(
	n int,
	name string,
) Step {
	return planned(newStep(n, name, PreOpen, 0, 0), Plan{RewritesState: true})
}

// A completed migration supersedes the image the one before it restored to, so
// the boot after it keeps the newest backup alone.
// boot is one boot of b's home that runs impls and lets the home go.
func boot(
	tb testing.TB,
	b *bench,
	impls Impls,
) {
	tb.Helper()
	s := b.begin()
	require.NoError(tb, s.RunPreOpen(context.Background(), impls))
	require.NoError(tb, s.Close())
}

func TestBegin_KeepsOnlyTheNewestBackupOnceTheMigrationAfterItCompleted(t *testing.T) {
	b := newBench(t, rewriting(1, "a"))
	testutil.WriteFile(t, b.home, "state/view.db", "first image")
	boot(t, b, tracing(new([]int), 1))
	first := ofKind(b.records(), kindBackup)[0].Path

	later, err := NewCatalog(rewriting(1, "a"), rewriting(2, "b"))
	require.NoError(t, err)
	b.cat = later
	testutil.WriteFile(t, b.home, "state/view.db", "second image")
	boot(t, b, tracing(new([]int), 2))
	second := ofKind(b.records(), kindBackup)[1].Path
	assert.FileExists(t, first, "the boot that took the second backup prunes nothing yet")

	b.begin()

	assert.NoFileExists(t, first)
	assert.Equal(t, "second image", backedUpView(t, second))
	assert.FileExists(t, filepath.Join(b.home.MigrateDir(), backupsDirName, restoreNoteName))
}

// While the last run failed, was interrupted or never ended, an older backup
// may be the only image a person can still go back to.
func TestBegin_KeepsEveryBackupWhileTheLastRunDidNotComplete(t *testing.T) {
	for name, end := range map[string][]record{
		"failed":      {{Kind: kindRunEnd, At: minute(3), Run: "r2", To: intPtr(0), Outcome: outcomeFailed}},
		"interrupted": {{Kind: kindRunEnd, At: minute(3), Run: "r2", To: intPtr(0), Outcome: outcomeInterrupted}},
		"dangling":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
			testutil.WriteFile(t, b.home, "migrate/backups/20261015T010203.000000000Z", "")
			testutil.WriteFile(t, b.home, "migrate/backups/20261016T010203.000000000Z.tar.gz", "older")
			testutil.WriteFile(t, b.home, "migrate/backups/20261017T010203.000000000Z.tar.gz", "newer")
			b.seedLedger(append([]record{
				{Kind: kindRunStart, At: minute(0), Run: "r1", From: intPtr(0), Pending: []int{1}},
				{Kind: kindRunEnd, At: minute(1), Run: "r1", To: intPtr(0), Outcome: outcomeCompleted},
				{Kind: kindRunStart, At: minute(2), Run: "r2", From: intPtr(0), Pending: []int{1}},
			}, end...)...)

			b.begin()

			listed, err := listBackups(b.home)
			require.NoError(t, err)
			assert.Equal(t, []string{
				filepath.Join(b.home.MigrateDir(), backupsDirName, "20261016T010203.000000000Z.tar.gz"),
				filepath.Join(b.home.MigrateDir(), backupsDirName, "20261017T010203.000000000Z.tar.gz"),
			}, listed, "a file without the archive suffix is not a backup, and nothing is pruned")
		})
	}
}

func TestBegin_PrunesTheDirectoryBackupsOfEarlierBuildsToo(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	testutil.WriteFile(t, b.home, "migrate/backups/20261016T010203.000000000Z/state/view.db", "earlier build")
	testutil.WriteFile(t, b.home, "migrate/backups/20261017T010203.000000000Z.tar.gz", "newest")
	b.seedLedger(
		record{Kind: kindRunStart, At: minute(0), Run: "r1", From: intPtr(0), Pending: []int{1}},
		record{Kind: kindStepDone, At: minute(1), Run: "r1", Step: 1, Name: "a", Outcome: outcomeApplied},
		record{Kind: kindRunEnd, At: minute(2), Run: "r1", To: intPtr(0), Outcome: outcomeCompleted},
	)

	b.begin()

	assert.NoDirExists(t, filepath.Join(b.home.MigrateDir(), backupsDirName, "20261016T010203.000000000Z"))
	assert.FileExists(t, filepath.Join(b.home.MigrateDir(), backupsDirName, "20261017T010203.000000000Z.tar.gz"))
}

// History answers from what the ledger held when the run began: the run in
// progress has not completed, so only a later boot sees it as one that did.
func TestHistory_CompletedAtSeesOnlyRunsThatEndedCompletedAtTheLayout(t *testing.T) {
	var seen []bool
	watch := newStep(2, "watch", PreOpen, 1, 1)
	watch.Recurs = true
	watch.Plan = func(_ context.Context, env Env) (Plan, error) {
		seen = append(seen, env.History.CompletedAt(1))
		return Plan{}, nil
	}
	b := newBench(t, newStep(1, "bump", PreOpen, 0, 1), watch)

	boot(t, b, tracing(new([]int), 1, 2))
	b.begin()

	assert.Equal(t, []bool{false, true}, seen)
}
