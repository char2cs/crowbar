package migrate

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestBegin_NothingPendingWritesNothing(t *testing.T) {
	t.Run("an empty catalog on a fresh home", func(t *testing.T) {
		b := newBench(t)
		s := b.begin()

		require.NoError(t, s.RunPreOpen(context.Background(), nil))
		require.NoError(t, s.RunPostOpen(context.Background(), nil))

		assert.NoFileExists(t, b.home.LayoutFile())
		assert.NoDirExists(t, b.home.MigrateDir())
		assert.Nil(t, s.Halt())
	})

	t.Run("a cancelled context with nothing pending", func(t *testing.T) {
		b := newBench(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := b.begin()

		require.NoError(t, s.RunPreOpen(ctx, nil))

		assert.NoDirExists(t, b.home.MigrateDir())
	})

	t.Run("a step that already ran in an earlier session", func(t *testing.T) {
		b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
		var order []int
		first := b.begin()
		require.NoError(t, first.RunPreOpen(context.Background(), tracing(&order, 1)))
		require.NoError(t, first.Close())

		second := b.begin()
		before := testutil.TreeSnapshot(t, b.home)
		require.NoError(t, second.RunPreOpen(context.Background(), tracing(&order, 1)))
		require.NoError(t, second.RunPostOpen(context.Background(), tracing(&order, 1)))

		assert.Equal(t, before, testutil.TreeSnapshot(t, b.home), "the ledger is not appended to")
		assert.Equal(t, []int{1}, order)
	})
}

func TestBegin_RefusesAHomeAnotherSessionHolds(t *testing.T) {
	b := newBench(t)
	first := b.begin()

	_, err := Begin(context.Background(), b.home, b.cat, Options{})
	require.ErrorIs(t, err, ErrHomeLocked)

	require.NoError(t, first.Close())
	b.begin()
}

// A home Begin cannot clean or record into halts like any other refusal: the
// daemon stays up holding the home, and its health says why.
func TestBegin_AHomeItCannotCleanOrRecordIntoHalts(t *testing.T) {
	tests := []struct {
		name  string
		perms bool
		write func(*bench)
		want  string
	}{
		{
			name:  "the backups directory is a file",
			write: func(b *bench) { testutil.WriteFile(b.tb, b.home, "migrate/backups", "a file, not a directory") },
			want:  "backup: list",
		},
		{
			name:  "a half-written backup that cannot be removed",
			perms: true,
			write: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "migrate/backups/20261017T010203.000000000Z.partial/state/x", "x")
				readOnly(b.tb, filepath.Join(b.home.MigrateDir(), backupsDirName, "20261017T010203.000000000Z.partial", "state"), 0o500)
			},
			want: "backup: remove",
		},
		{
			name:  "a run left open in a ledger that cannot be written",
			perms: true,
			write: func(b *bench) {
				b.seedLedger(record{Kind: kindRunStart, At: ledgerEpoch, Run: "r-killed", Pending: []int{1}, From: new(int)})
				readOnly(b.tb, filepath.Join(b.home.MigrateDir(), ledgerName), 0o400)
			},
			want: "close unfinished run r-killed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.perms {
				testutil.RequirePermissionEnforcement(t)
			}
			b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
			tc.write(b)
			var order []int

			s := b.begin()

			h := s.Halt()
			require.NotNil(t, h)
			assert.Equal(t, HaltIOFailed, h.Reason)
			assert.Contains(t, h.Message, tc.want)
			assert.Same(t, h, halted(t, s.RunPreOpen(context.Background(), tracing(&order, 1))))
			assert.Empty(t, order)
			_, err := lockHome(b.home)
			require.ErrorIs(t, err, ErrHomeLocked, "the halted session still holds the home")
		})
	}
}

// readOnly gives path mode until the test ends, then gives the owner back what
// the temp dir's cleanup needs.
func readOnly(
	tb testing.TB,
	path string,
	mode os.FileMode,
) {
	tb.Helper()
	require.NoError(tb, os.Chmod(path, mode))
	tb.Cleanup(func() { require.NoError(tb, os.Chmod(path, 0o700)) })
}

func TestBegin_RefusesANewerLayout(t *testing.T) {
	steps := []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PostOpen, 1, 1)}

	t.Run("a fresh newer home gets no migrate directory", func(t *testing.T) {
		b := newBench(t, steps...)
		testutil.WriteFile(t, b.home, "LAYOUT", "5\n")
		testutil.WriteFile(t, b.home, "state/.lock", "")
		before := testutil.TreeSnapshot(t, b.home)
		var order []int

		s := b.begin()

		h := s.Halt()
		require.NotNil(t, h)
		assert.Equal(t, HaltLayoutNewer, h.Reason)
		assert.Contains(t, h.Message, "layout 5")
		assert.Contains(t, h.Message, "layout 1")
		assert.Equal(t, 5, h.Layout)
		assert.Equal(t, 1, h.Known)
		assert.Nil(t, h.Step, "no step is to blame")
		assert.Same(t, h, halted(t, s.RunPreOpen(context.Background(), tracing(&order, 1, 2))))
		assert.Same(t, h, halted(t, s.RunPostOpen(context.Background(), tracing(&order, 1, 2))))
		assert.Empty(t, order)
		assert.NoDirExists(t, b.home.MigrateDir(), "an older binary must not write into a newer home")
		assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
		raw, err := os.ReadFile(b.home.LayoutFile())
		require.NoError(t, err)
		assert.Equal(t, "5\n", string(raw))
		_, err = lockHome(b.home)
		require.ErrorIs(t, err, ErrHomeLocked, "the refused session still holds the home")
	})

	t.Run("a crashed run and half-written backups are left as they are", func(t *testing.T) {
		b := newBench(t, steps...)
		testutil.WriteFile(t, b.home, "LAYOUT", "5\n")
		testutil.WriteFile(t, b.home, "migrate/backups/20261017T010203.000000000Z.partial/state/x", "x")
		testutil.WriteFile(t, b.home, "state/.lock", "")
		b.seedLedger(
			record{Kind: kindRunStart, At: ledgerEpoch, Run: "r-newer", Pending: []int{1}},
			record{Kind: kindStepStart, At: ledgerEpoch, Run: "r-newer", Step: 1, Name: "a"},
		)
		before := testutil.TreeSnapshot(t, b.home)

		s := b.begin()

		require.NotNil(t, s.Halt())
		assert.Equal(t, HaltLayoutNewer, s.Halt().Reason)
		assert.Equal(t, filepath.Join(b.home.MigrateDir(), ledgerName), s.Halt().Ledger)
		assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
		assert.Equal(t, []string{kindRunStart, kindStepStart}, b.kinds(), "the newer binary's run is not ours to close")
	})
}

func TestBegin_UnreadableLayoutHaltsAndNeverRunsAnyStep(t *testing.T) {
	tests := []struct {
		name  string
		write func(*bench)
	}{
		{"empty", func(b *bench) { testutil.WriteFile(b.tb, b.home, "LAYOUT", "") }},
		{"not a number", func(b *bench) { testutil.WriteFile(b.tb, b.home, "LAYOUT", "abc\n") }},
		{"negative", func(b *bench) { testutil.WriteFile(b.tb, b.home, "LAYOUT", "-1\n") }},
		{"a directory", func(b *bench) { require.NoError(b.tb, os.Mkdir(b.home.LayoutFile(), 0o700)) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			b := newBench(t, newStep(1, "legacy", PreOpen, 0, 0), newStep(2, "bump", PreOpen, 0, 1))
			tc.write(b)
			testutil.WriteFile(t, b.home, "state/.lock", "")
			before := testutil.TreeSnapshot(t, b.home)
			var order []int

			s := b.begin()

			h := s.Halt()
			require.NotNil(t, h)
			assert.Equal(t, HaltLayoutUnreadable, h.Reason)
			assert.Contains(t, h.Message, "LAYOUT")
			assert.Equal(t, 1, h.Known)
			assert.Same(t, h, halted(t, s.RunPreOpen(ctx, tracing(&order, 1, 2))))
			assert.Empty(t, order, "garbage is not a legacy layout")
			assert.NoDirExists(t, b.home.MigrateDir())
			assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
		})
	}
}

func TestBegin_ALedgerItCannotTrustHalts(t *testing.T) {
	tests := []struct {
		name  string
		write func(*bench)
		want  string
	}{
		{
			name: "garbage in the middle",
			write: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "migrate/ledger.jsonl",
					`{"v":1,"kind":"run.start","run":"r1"}`+"\n"+"garbage\n"+`{"v":1,"kind":"run.end","run":"r1"}`+"\n")
			},
			want: "line 2",
		},
		{
			name: "a ledger that cannot be read",
			write: func(b *bench) {
				require.NoError(b.tb, os.MkdirAll(filepath.Join(b.home.MigrateDir(), ledgerName), 0o700))
			},
			want: "is a directory",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
			tc.write(b)
			testutil.WriteFile(t, b.home, "migrate/backups/20261017T010203.000000000Z.partial/state/x", "x")
			testutil.WriteFile(t, b.home, "state/.lock", "")
			before := testutil.TreeSnapshot(t, b.home)
			var order []int

			s := b.begin()

			h := s.Halt()
			require.NotNil(t, h)
			assert.Equal(t, HaltLedgerCorrupt, h.Reason)
			assert.Contains(t, h.Message, tc.want)
			assert.Equal(t, filepath.Join(b.home.MigrateDir(), ledgerName), h.Ledger)
			assert.Same(t, h, halted(t, s.RunPreOpen(context.Background(), tracing(&order, 1))))
			assert.Empty(t, order)
			assert.Equal(t, before, testutil.TreeSnapshot(t, b.home), "a ledger nobody can read is left for a person")
			_, err := lockHome(b.home)
			require.ErrorIs(t, err, ErrHomeLocked)
		})
	}
}

func TestBegin_ClosesADanglingRunAsInterrupted(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	b.seedLedger(
		record{Kind: kindRunStart, At: ledgerEpoch, Run: "r-killed", Pending: []int{1}, From: new(int)},
		record{Kind: kindStepStart, At: ledgerEpoch, Run: "r-killed", Step: 1, Name: "a", Phase: "pre-open"},
	)

	s := b.begin()

	records := b.records()
	require.Equal(t, []string{kindRunStart, kindStepStart, kindRunEnd}, kindsOf(records))
	end := records[2]
	assert.Equal(t, "r-killed", end.Run)
	assert.Equal(t, outcomeInterrupted, end.Outcome)
	assert.Equal(t, 0, layoutOf(end.To))
	assert.NotEmpty(t, end.Error)
	assert.Equal(t, []int{1}, numbers(s.pending()), "closing the run does not complete its step")
	assert.Nil(t, s.Halt())

	var order []int
	require.NoError(t, s.RunPreOpen(ctx, tracing(&order, 1)))
	assert.Equal(t, []int{1}, order, "the next run does the work the killed one did not finish")
	assert.Len(t, ofKind(b.records(), kindRunStart), 2)
}

func TestBegin_ABumpWhoseLayoutLandedBeforeItsDoneRecordIsNotRerun(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 1))
	testutil.WriteFile(t, b.home, "LAYOUT", "1\n")
	b.seedLedger(
		record{Kind: kindRunStart, At: ledgerEpoch, Run: "r-killed", Pending: []int{1}, From: new(int)},
		record{Kind: kindStepStart, At: ledgerEpoch, Run: "r-killed", Step: 1, Name: "a", Phase: "pre-open"},
	)
	var order []int
	s := b.begin()

	require.NoError(t, s.RunPreOpen(ctx, tracing(&order, 1)))
	require.NoError(t, s.RunPostOpen(ctx, tracing(&order, 1)))

	assert.Empty(t, order, "LAYOUT alone decides a bump is done")
	records := b.records()
	require.Equal(t, []string{kindRunStart, kindStepStart, kindRunEnd}, kindsOf(records), "no new run for nothing to do")
	assert.Equal(t, "r-killed", records[2].Run)
	assert.Equal(t, outcomeCompleted, records[2].Outcome, "the bump landed, so the run had nothing left to do")
	assert.Equal(t, 1, layoutOf(records[2].To))
}

func TestBegin_ClosesADanglingRunByWhatItLeftUndone(t *testing.T) {
	tests := []struct {
		name    string
		pending []int
		want    string
	}{
		{name: "every step of the run is done", pending: []int{1, 2}, want: outcomeCompleted},
		{name: "a step the catalog no longer has", pending: []int{1, 9}, want: outcomeInterrupted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PreOpen, 0, 0))
			recs := []record{{Kind: kindRunStart, At: ledgerEpoch, Run: "r-killed", Pending: tc.pending, From: new(int)}}
			for i, name := range []string{"a", "b"} {
				recs = append(recs, record{
					Kind: kindStepDone, At: ledgerEpoch, Run: "r-killed", Step: i + 1, Name: name, Outcome: outcomeApplied,
				})
			}
			b.seedLedger(recs...)

			b.begin()

			records := b.records()
			end := records[len(records)-1]
			assert.Equal(t, kindRunEnd, end.Kind)
			assert.Equal(t, "r-killed", end.Run)
			assert.Equal(t, tc.want, end.Outcome)
		})
	}
}

func TestBegin_AFinishedDanglingRunRetiresItsBackup(t *testing.T) {
	rewrites := func(n int, name string) Step {
		return planned(newStep(n, name, PreOpen, 0, 0), Plan{RewritesState: true})
	}
	b := newBench(t, rewrites(1, "a"), rewrites(2, "b"))
	testutil.WriteFile(t, b.home, "state/view.db", "the rows now")
	testutil.WriteFile(t, b.home, "migrate/backups/r-old/state/view.db", "the rows long ago")
	old := filepath.Join(b.home.MigrateDir(), backupsDirName, "r-old")
	b.seedLedger(
		record{Kind: kindRunStart, At: ledgerEpoch, Run: "r-old", Pending: []int{1}, From: new(int)},
		record{Kind: kindBackup, At: ledgerEpoch, Run: "r-old", Path: old, Bytes: 17},
		record{Kind: kindStepStart, At: ledgerEpoch, Run: "r-old", Step: 1, Name: "a", Phase: "pre-open"},
		record{Kind: kindStepDone, At: ledgerEpoch, Run: "r-old", Step: 1, Name: "a", Phase: "pre-open", Outcome: outcomeApplied},
	)
	var order []int
	s := b.begin()

	require.NoError(t, s.RunPreOpen(context.Background(), tracing(&order, 1, 2)))

	assert.Equal(t, []int{2}, order)
	backups := ofKind(b.records(), kindBackup)
	require.Len(t, backups, 2, "the finished run's backup is not the image a later migration starts from")
	assert.NotEqual(t, old, backups[1].Path)
	raw, err := os.ReadFile(filepath.Join(backups[1].Path, "state", "view.db"))
	require.NoError(t, err)
	assert.Equal(t, "the rows now", string(raw))
}

func TestBegin_RemovesHalfWrittenBackups(t *testing.T) {
	b := newBench(t)
	testutil.WriteFile(t, b.home, "migrate/backups/20261017T010203.000000000Z.partial/state/x", "x")
	testutil.WriteFile(t, b.home, "migrate/backups/20261016T010203.000000000Z/state/y", "y")

	b.begin()

	assert.NoDirExists(t, filepath.Join(b.home.MigrateDir(), "backups", "20261017T010203.000000000Z.partial"))
	assert.FileExists(t, filepath.Join(b.home.MigrateDir(), "backups", "20261016T010203.000000000Z", "state", "y"))
}

func TestSession_CloseReleasesTheHomeAndWritesNothing(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	s := b.begin()
	require.NoError(t, s.RunPreOpen(context.Background(), healthy(1)))
	before := testutil.TreeSnapshot(t, b.home)

	require.NoError(t, s.Close())
	require.NoError(t, s.Close(), "closing twice is fine")

	assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
	lock, err := lockHome(b.home)
	require.NoError(t, err)
	require.NoError(t, lock.Close())
}

func TestSession_CloseWithoutALockIsSafe(t *testing.T) {
	require.NoError(t, (&Session{}).Close())
}

func TestOptions_DefaultsProbeTheRealSystem(t *testing.T) {
	opts := Options{}.withDefaults()

	assert.WithinDuration(t, time.Now(), opts.Clock(), time.Minute)
	free, err := opts.FreeBytes(t.TempDir())
	require.NoError(t, err)
	assert.NotZero(t, free)
	assert.Same(t, slog.Default(), opts.Log)
}
