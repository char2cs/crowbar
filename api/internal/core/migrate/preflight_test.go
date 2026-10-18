package migrate

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

// planned is step with the plan p; its Plan ignores the context.
func planned(
	step Step,
	p Plan,
) Step {
	step.Plan = func(context.Context, Env) (Plan, error) {
		return p, nil
	}
	return step
}

func stateFiles(
	tb testing.TB,
	b *bench,
) []string {
	tb.Helper()
	var out []string
	for _, line := range testutil.TreeSnapshot(tb, b.home) {
		if strings.HasPrefix(line, "state/") {
			out = append(out, line)
		}
	}
	return out
}

func TestRun_PlanErrorHalts(t *testing.T) {
	failing := newStep(2, "b", PostOpen, 0, 0)
	failing.Plan = func(context.Context, Env) (Plan, error) {
		return Plan{}, errors.New("cannot count rows")
	}
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), failing)
	var order []int
	s := b.begin()

	err := s.RunPreOpen(context.Background(), tracing(&order, 1, 2))

	h := halted(t, err)
	assert.Same(t, h, s.Halt())
	assert.Equal(t, HaltPlanFailed, h.Reason)
	require.NotNil(t, h.Step)
	assert.Equal(t, ID{Number: 2, Name: "b"}, *h.Step, "every pending step is planned, whichever its phase")
	assert.Equal(t, "post-open", h.Phase)
	assert.Contains(t, h.Message, "cannot count rows")
	assert.Empty(t, order, "no step runs when any plan fails")
	records := b.records()
	assert.Equal(t, []string{kindRunStart, kindRunEnd}, kindsOf(records), "no step ran, so no step failed")
	assert.Equal(t, outcomeFailed, records[1].Outcome)
	assert.Equal(t, h.Run, records[1].Run)
}

func TestRun_PreflightGit(t *testing.T) {
	needsGit := planned(newStep(2, "b", PostOpen, 0, 0), Plan{NeedsGit: true, RewritesState: true})
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), needsGit)
	testutil.WriteFile(t, b.home, "state/view.db", "rows")
	var order []int

	t.Run("without git", func(t *testing.T) {
		b.haveGit = func() bool { return false }
		s := b.begin()

		err := s.RunPreOpen(context.Background(), tracing(&order, 1, 2))

		h := halted(t, err)
		assert.Equal(t, HaltPreflight, h.Reason)
		assert.Contains(t, h.Message, "git")
		require.NotNil(t, h.Step)
		assert.Equal(t, 2, h.Step.Number, "the step that needs git")
		assert.Empty(t, order)
		assert.NoDirExists(t, filepath.Join(b.home.MigrateDir(), backupsDirName))
		assert.Equal(t, []string{kindRunStart, kindRunEnd}, b.kinds())
		require.NoError(t, s.Close())
	})

	t.Run("with git", func(t *testing.T) {
		b.haveGit = func() bool { return true }

		require.NoError(t, b.begin().RunPreOpen(context.Background(), tracing(&order, 1)))

		assert.Equal(t, []int{1}, order)
	})
}

func TestRun_BacksUpStateBeforeTheFirstStepWhenAPlanRewritesIt(t *testing.T) {
	ctx := context.Background()
	b := newBench(t,
		newStep(1, "a", PreOpen, 0, 0),
		planned(newStep(2, "b", PostOpen, 0, 0), Plan{RewritesState: true}),
	)
	testutil.WriteFile(t, b.home, "state/view.db", "the rows before")
	var backedUp string
	impls := Impls{
		1: impl(func(*Checkpoints) (Counts, error) {
			run := b.records()[0].Run
			raw, err := os.ReadFile(filepath.Join(b.home.MigrateDir(), backupsDirName, run, "state", "view.db"))
			require.NoError(t, err, "the backup exists before step 1 starts, though step 2 is the one that rewrites")
			backedUp = string(raw)
			return nil, nil
		}),
		2: impl(func(*Checkpoints) (Counts, error) {
			return nil, errors.New("boom")
		}),
	}
	s := b.begin()
	require.NoError(t, s.RunPreOpen(ctx, impls))

	err := s.RunPostOpen(ctx, impls)

	assert.Equal(t, "the rows before", backedUp)
	h := halted(t, err)
	records := b.records()
	want := filepath.Join(b.home.MigrateDir(), backupsDirName, records[0].Run)
	assert.Equal(t, want, h.Backup)
	assert.Contains(t, h.Message, want, "the halt names the backup to restore from")
	assert.Equal(t, []string{
		kindRunStart, kindBackup, kindStepStart, kindStepDone, kindStepStart, kindStepFail, kindRunEnd,
	}, kindsOf(records))
	backup := records[1]
	assert.Equal(t, want, backup.Path)
	assert.Equal(t, int64(len("the rows before")), backup.Bytes)
	assert.Equal(t, records[0].Run, backup.Run)
	assert.NoFileExists(t, filepath.Join(want, "state", homeLockName), "the lock file is not state")
}

func TestRun_NoBackupWhenNoPlanRewritesStateOrStateIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		plan  Plan
		state string
	}{
		{name: "no plan rewrites state", plan: Plan{NeedsGit: true}, state: "rows"},
		{name: "state is empty", plan: Plan{RewritesState: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, planned(newStep(1, "a", PreOpen, 0, 0), tc.plan))
			if tc.state != "" {
				testutil.WriteFile(t, b.home, "state/view.db", tc.state)
			}
			var order []int
			s := b.begin()

			require.NoError(t, s.RunPreOpen(context.Background(), tracing(&order, 1)))

			assert.Equal(t, []int{1}, order)
			assert.Empty(t, ofKind(b.records(), kindBackup))
			assert.NoDirExists(t, filepath.Join(b.home.MigrateDir(), backupsDirName))
		})
	}
}

func TestRun_InsufficientDiskHaltsBeforeAnyStep(t *testing.T) {
	b := newBench(t, planned(newStep(1, "a", PreOpen, 0, 0), Plan{RewritesState: true}))
	testutil.WriteFile(t, b.home, "state/view.db", "the rows before")
	b.free = diskOf(1)
	var order []int
	s := b.begin()
	before := stateFiles(t, b)

	err := s.RunPreOpen(context.Background(), tracing(&order, 1))

	h := halted(t, err)
	assert.Equal(t, HaltPreflight, h.Reason)
	assert.Contains(t, h.Message, "free")
	require.NotNil(t, h.Step)
	assert.Equal(t, 1, h.Step.Number)
	assert.Empty(t, order)
	assert.Equal(t, before, stateFiles(t, b))
	assert.Empty(t, h.Backup)
	assertNoBackupFor(t, b.home, h.Run)
	assert.Equal(t, []string{kindRunStart, kindRunEnd}, b.kinds())
}

func TestRun_StateThatCannotBeBackedUpHaltsBeforeAnyStep(t *testing.T) {
	b := newBench(t, planned(newStep(1, "a", PreOpen, 0, 0), Plan{RewritesState: true}))
	testutil.WriteFile(t, b.home, "state/view.db", "rows")
	symlinkOrSkip(t, filepath.Join(b.home.State(), "view.db"), filepath.Join(b.home.State(), "alias"))
	var order []int
	s := b.begin()

	err := s.RunPreOpen(context.Background(), tracing(&order, 1))

	h := halted(t, err)
	assert.Equal(t, HaltPreflight, h.Reason)
	assert.Contains(t, h.Message, "symlink")
	assert.Empty(t, order)
	assertNoBackupFor(t, b.home, h.Run)
}

func TestRun_OneBackupPerMigrationAttempt(t *testing.T) {
	ctx := context.Background()
	rewrites := planned(newStep(1, "a", PreOpen, 0, 0), Plan{RewritesState: true})
	b := newBench(t, rewrites)
	testutil.WriteFile(t, b.home, "state/view.db", "the rows before")
	attempts := 0
	flaky := Impls{1: impl(func(*Checkpoints) (Counts, error) {
		attempts++
		testutil.WriteFile(t, b.home, "state/view.db", "half migrated")
		if attempts < 3 {
			return nil, errors.New("boom")
		}
		return nil, nil
	})}
	first := b.begin()
	firstHalt := halted(t, first.RunPreOpen(ctx, flaky))
	require.NotEmpty(t, firstHalt.Backup)
	require.NoError(t, first.Close())

	second := b.begin()
	secondHalt := halted(t, second.RunPreOpen(ctx, flaky))

	assert.Equal(t, firstHalt.Backup, secondHalt.Backup, "the retry's halt points at the pre-migration image")
	assert.NotEqual(t, firstHalt.Run, secondHalt.Run)
	assert.Len(t, ofKind(b.records(), kindBackup), 1, "a retry takes no second backup")
	raw, err := os.ReadFile(filepath.Join(firstHalt.Backup, "state", "view.db"))
	require.NoError(t, err)
	assert.Equal(t, "the rows before", string(raw), "partially migrated state never overwrites the image")
	entries, err := os.ReadDir(filepath.Join(b.home.MigrateDir(), backupsDirName))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	require.NoError(t, second.Close())

	third := b.begin()
	require.NoError(t, third.RunPreOpen(ctx, flaky))
	require.NoError(t, third.Close())
	assert.Len(t, ofKind(b.records(), kindBackup), 1)

	// A completed run ends the attempt: the next migration backs up what it starts from.
	later, err := NewCatalog(rewrites, planned(newStep(2, "b", PreOpen, 0, 0), Plan{RewritesState: true}))
	require.NoError(t, err)
	b.cat = later
	require.NoError(t, b.begin().RunPreOpen(ctx, Impls{2: flaky[1]}))
	backups := ofKind(b.records(), kindBackup)
	require.Len(t, backups, 2)
	assert.NotEqual(t, backups[0].Path, backups[1].Path)
	raw, err = os.ReadFile(filepath.Join(backups[1].Path, "state", "view.db"))
	require.NoError(t, err)
	assert.Equal(t, "half migrated", string(raw))
}

func TestRun_ABackupSomeoneRemovedIsTakenAgain(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, planned(newStep(1, "a", PreOpen, 0, 0), Plan{RewritesState: true}))
	var logs bytes.Buffer
	b.log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	testutil.WriteFile(t, b.home, "state/view.db", "rows")
	failing := Impls{1: impl(func(*Checkpoints) (Counts, error) {
		return nil, errors.New("boom")
	})}
	first := b.begin()
	firstHalt := halted(t, first.RunPreOpen(ctx, failing))
	require.NoError(t, first.Close())
	require.NoError(t, os.RemoveAll(firstHalt.Backup))

	secondHalt := halted(t, b.begin().RunPreOpen(ctx, failing))

	assert.NotEqual(t, firstHalt.Backup, secondHalt.Backup, "a backup that is gone is no backup")
	assert.DirExists(t, secondHalt.Backup)
	assert.Contains(t, logs.String(), firstHalt.Backup, "the loss of the first image is not silent")
}

func TestRun_ABackupTheLedgerCannotRecordHaltsAndIsNotInForce(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, planned(newStep(1, "a", PreOpen, 0, 0), Plan{RewritesState: true}))
	testutil.WriteFile(t, b.home, "state/view.db", "rows")
	var s *Session
	tick := tickingClock()
	b.clock = func() time.Time {
		if s != nil && s.run != nil {
			require.NoError(t, s.led.Close()) // run.start is down; the backup record is next
		}
		return tick()
	}
	s = b.begin()
	var order []int

	err := s.RunPreOpen(ctx, tracing(&order, 1))

	h := halted(t, err)
	assert.Equal(t, HaltPreflight, h.Reason)
	copied := filepath.Join(b.home.MigrateDir(), backupsDirName, h.Run)
	assert.NoDirExists(t, copied, "a copy the ledger never knew of would never be offered or reused")
	assert.Empty(t, h.Backup, "no durable record, so nothing is in force")
	assert.Empty(t, order)
	require.NoError(t, s.Close())

	b.clock = tick
	require.NoError(t, b.begin().RunPreOpen(ctx, tracing(&order, 1)))
	backups := ofKind(b.records(), kindBackup)
	require.Len(t, backups, 1)
	assert.NotEqual(t, copied, backups[0].Path, "the next boot backs up afresh")
	assert.Equal(t, []int{1}, order)
}

// sessionless is a Halt without what only a running session knows when the
// preview is made: the run it started and the ledger file that run created.
func sessionless(
	h *Halt,
) Halt {
	c := *h
	c.Run, c.Ledger = "", ""
	return c
}

// A preview that is green where the boot halts is worse than none: Inspect makes
// the preflight's decisions through the code the session runs, so its Halt is the
// boot's.
func TestInspect_APreflightFailureIsTheHaltTheBootGives(t *testing.T) {
	rewrites := Plan{Actions: []Action{{Kind: Convert, Subject: "state/x", Count: Unknown}}, RewritesState: true}
	tests := []struct {
		name      string
		plan      func(context.Context, Env) (Plan, error)
		arrange   func(*bench)
		reason    HaltReason
		message   string
		hasBackup bool
	}{
		{
			name:   "a step whose plan fails",
			plan:   func(context.Context, Env) (Plan, error) { return Plan{}, errors.New("view.db is locked") },
			reason: HaltPlanFailed, message: "view.db is locked",
		},
		{
			name: "a symlink in state",
			plan: planning(rewrites),
			arrange: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "state/a.db", "rows")
				symlinkOrSkip(b.tb, filepath.Join(b.home.State(), "a.db"), filepath.Join(b.home.State(), "alias.db"))
			},
			reason: HaltPreflight, message: "unsupported symlink",
		},
		{
			name: "no room for the copy",
			plan: planning(rewrites),
			arrange: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "state/a.db", "rows")
				b.free = diskOf(1)
			},
			reason: HaltPreflight, message: "not enough free disk space",
		},
		{
			name: "free space that cannot be probed",
			plan: planning(rewrites),
			arrange: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "state/a.db", "rows")
				b.free = func(string) (uint64, error) { return 0, errors.New("statfs: no such device") }
			},
			reason: HaltPreflight, message: "statfs: no such device",
		},
		{
			name:    "git missing",
			plan:    planning(Plan{NeedsGit: true}),
			arrange: func(b *bench) { b.haveGit = func() bool { return false } },
			reason:  HaltPreflight, message: "git is required",
		},
		{
			name: "git missing on a retry that keeps an earlier backup",
			plan: planning(Plan{NeedsGit: true, RewritesState: true}),
			arrange: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "state/a.db", "rows")
				s := b.begin()
				boom := Impls{1: impl(func(*Checkpoints) (Counts, error) { return nil, errors.New("boom") })}
				assert.Equal(b.tb, HaltStepFailed, halted(b.tb, s.RunPreOpen(context.Background(), boom)).Reason)
				require.NoError(b.tb, s.Close())
				b.haveGit = func() bool { return false }
			},
			reason: HaltPreflight, message: "git is required", hasBackup: true,
		},
		{
			name: "git missing after a finished dangling run whose backup is on disk",
			plan: planning(Plan{NeedsGit: true}),
			arrange: func(b *bench) {
				backup := filepath.Join(b.home.MigrateDir(), backupsDirName, "r1", "state")
				testutil.WriteFile(b.tb, b.home, "migrate/backups/r1/state/a.db", "old")
				b.seedLedger(
					record{Kind: kindRunStart, At: minute(0), Run: "r1", From: intPtr(0), Pending: []int{1}},
					record{Kind: kindBackup, At: minute(1), Run: "r1", Path: backup, Bytes: 3},
					record{Kind: kindStepDone, At: minute(2), Run: "r1", Step: 1, Name: "a", Outcome: outcomeApplied},
				)
				b.haveGit = func() bool { return false }
			},
			reason: HaltPreflight, message: "git is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			step := newStep(2, "b", PostOpen, 0, 0)
			step.Plan = tc.plan
			b := newBench(t, newStep(1, "a", PreOpen, 0, 0), step)
			if tc.arrange != nil {
				tc.arrange(b)
			}
			before := testutil.TreeSnapshot(t, b.home)

			_, err := b.inspect()

			got := halted(t, err)
			assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
			assert.Equal(t, tc.reason, got.Reason)
			assert.Contains(t, got.Message, tc.message)
			assert.Equal(t, &ID{Number: 2, Name: "b"}, got.Step)
			assert.Equal(t, "post-open", got.Phase)
			assert.Equal(t, tc.hasBackup, got.Backup != "", "a retry's halt names the backup it keeps")
			boot := b.begin().RunPreOpen(context.Background(), healthy(1, 2))
			assert.Equal(t, sessionless(halted(t, boot)), sessionless(got), "the preview halts as the boot does")
		})
	}
}
