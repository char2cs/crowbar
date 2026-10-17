package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

// planning is a Plan func that returns p.
func planning(
	p Plan,
) func(context.Context, Env) (Plan, error) {
	return func(context.Context, Env) (Plan, error) {
		return p, nil
	}
}

func planStep(
	number int,
	name string,
	phase Phase,
	from int,
	to int,
	p Plan,
) Step {
	s := newStep(number, name, phase, from, to)
	s.Plan = planning(p)
	return s
}

// inspect runs Inspect over the bench's home with its injected machine.
func (b *bench) inspect() (Report, error) {
	b.tb.Helper()
	return Inspect(context.Background(), b.home, b.cat, Options{FreeBytes: b.free, HaveGit: b.haveGit})
}

func TestInspect_IsReadOnly(t *testing.T) {
	steps := []Step{
		planStep(1, "a", PreOpen, 0, 0, Plan{Actions: []Action{{Kind: Convert, Subject: "state/x", Count: 1}}, RewritesState: true}),
		planStep(2, "b", PreOpen, 0, 1, Plan{}),
		planStep(3, "c", PostOpen, 1, 1, Plan{}),
	}

	t.Run("a home with state, a ledger and steps pending", func(t *testing.T) {
		b := newBench(t, steps...)
		testutil.WriteFile(t, b.home, "state/x.db", "database bytes")
		b.seedLedger(
			record{Kind: kindRunStart, At: ledgerEpoch, Run: "r1", From: intPtr(0), Pending: []int{1, 2, 3}},
			record{Kind: kindRunEnd, At: ledgerEpoch, Run: "r1", To: intPtr(0), Outcome: outcomeInterrupted},
		)
		before := testutil.TreeSnapshot(t, b.home)

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Len(t, report.Steps, 3)
		assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
		assert.NoFileExists(t, b.home.LayoutFile())
		assert.NoFileExists(t, b.home.LockFile())
		assert.NoDirExists(t, filepath.Join(b.home.MigrateDir(), backupsDirName))
	})

	t.Run("a dangling run is not closed", func(t *testing.T) {
		b := newBench(t, steps...)
		b.seedLedger(record{Kind: kindRunStart, At: ledgerEpoch, Run: "r1", From: intPtr(0), Pending: []int{1}})
		before := testutil.TreeSnapshot(t, b.home)

		report, err := b.inspect()

		require.NoError(t, err)
		require.NotNil(t, report.Last)
		assert.True(t, report.Last.End.IsZero())
		assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
		assert.Equal(t, []string{kindRunStart}, b.kinds())
	})

	t.Run("a home without state or migrate directories", func(t *testing.T) {
		b := newBench(t, steps...)

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, numbers(stepsOf(report)))
		assert.Empty(t, testutil.TreeSnapshot(t, b.home))
		assert.NoDirExists(t, b.home.State())
		assert.NoDirExists(t, b.home.MigrateDir())
		assert.False(t, report.LockHeld)
		assert.Nil(t, report.Last)
	})

	t.Run("a home directory that does not exist", func(t *testing.T) {
		b := newBench(t, steps...)
		require.NoError(t, os.Remove(b.home.Root()))

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Len(t, report.Steps, 3)
		assert.NoDirExists(t, b.home.Root())
	})
}

func stepsOf(
	r Report,
) []Step {
	out := make([]Step, 0, len(r.Steps))
	for _, sp := range r.Steps {
		out = append(out, sp.Step)
	}
	return out
}

func TestInspect_ListsWhatASessionWouldRun(t *testing.T) {
	b := newBench(t,
		planStep(1, "a", PreOpen, 0, 0, Plan{Actions: []Action{{Kind: Delete, Subject: "x", Count: 2}}}),
		newStep(2, "b", PreOpen, 0, 1),
		newStep(3, "c", PreOpen, 1, 1),
		newStep(4, "d", PostOpen, 1, 1),
	)
	b.seedLedger(
		record{Kind: kindRunStart, At: ledgerEpoch, Run: "r1", From: intPtr(0), Pending: []int{1, 2, 3, 4}},
		record{Kind: kindStepDone, At: ledgerEpoch, Run: "r1", Step: 3, Name: "c", Outcome: outcomeApplied},
		record{Kind: kindRunEnd, At: ledgerEpoch, Run: "r1", To: intPtr(0), Outcome: outcomeInterrupted},
	)

	report, err := b.inspect()

	require.NoError(t, err)
	assert.Equal(t, b.home.Root(), report.Home)
	assert.Equal(t, 0, report.Layout)
	assert.Equal(t, 1, report.Known)
	assert.Equal(t, []Action{{Kind: Delete, Subject: "x", Count: 2}}, report.Steps[0].Plan.Actions)
	var order []int
	s := b.begin()
	require.NoError(t, s.RunPreOpen(context.Background(), tracing(&order, 1, 2, 3, 4)))
	require.NoError(t, s.RunPostOpen(context.Background(), tracing(&order, 1, 2, 3, 4)))
	assert.Equal(t, order, numbers(stepsOf(report)), "Inspect and a session share one pending computation")
	assert.Equal(t, []int{1, 2, 4}, order, "the applied in-layout step is skipped")
}

func TestInspect_ReportsLockHeld(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	s := b.begin()

	report, err := b.inspect()

	require.NoError(t, err)
	assert.True(t, report.LockHeld)

	require.NoError(t, s.Close())
	report, err = b.inspect()
	require.NoError(t, err)
	assert.False(t, report.LockHeld)
}

func TestInspect_RefusesAHomeBeginRefuses(t *testing.T) {
	steps := []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PostOpen, 1, 1)}
	tests := []struct {
		name   string
		write  func(*bench)
		reason HaltReason
	}{
		{"a newer layout", func(b *bench) { testutil.WriteFile(b.tb, b.home, "LAYOUT", "9\n") }, HaltLayoutNewer},
		{"an unreadable layout", func(b *bench) { testutil.WriteFile(b.tb, b.home, "LAYOUT", "abc\n") }, HaltLayoutUnreadable},
		{"a corrupt ledger", func(b *bench) {
			testutil.WriteFile(b.tb, b.home, "migrate/ledger.jsonl", "{\"v\":1,\"kind\":\"run.start\"}\ngarbage\n{\"v\":1,\"kind\":\"run.end\"}\n")
		}, HaltLedgerCorrupt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, steps...)
			tc.write(b)
			before := testutil.TreeSnapshot(t, b.home)

			_, err := b.inspect()

			got := halted(t, err)
			assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
			assert.Equal(t, tc.reason, got.Reason)
			assert.Equal(t, 1, got.Known)
			want := b.begin().Halt()
			require.NotNil(t, want)
			assert.Equal(t, want, got, "the preview refuses with the halt the boot would")
		})
	}
}

func TestInspect_NewerLayoutReturnsHalt(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 1))
	testutil.WriteFile(t, b.home, "LAYOUT", "9\n")

	_, err := b.inspect()

	got := halted(t, err)
	assert.Equal(t, HaltLayoutNewer, got.Reason)
	assert.Equal(t, 9, got.Layout)
	assert.Contains(t, got.Message, "layout 9")
}

// sessionless is a Halt without what only a running session knows: its run, the
// ledger it wrote and the backup it took.
func sessionless(
	h *Halt,
) Halt {
	c := *h
	c.Run, c.Ledger, c.Backup = "", "", ""
	return c
}

// A preview that is green where the boot halts is worse than none: Inspect makes
// the preflight's decisions through the code the session runs, so its Halt is the
// boot's.
func TestInspect_APreflightFailureIsTheHaltTheBootGives(t *testing.T) {
	rewrites := Plan{Actions: []Action{{Kind: Convert, Subject: "state/x", Count: Unknown}}, RewritesState: true}
	tests := []struct {
		name    string
		plan    func(context.Context, Env) (Plan, error)
		arrange func(*bench)
		reason  HaltReason
		message string
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
			boot := b.begin().RunPreOpen(context.Background(), healthy(1, 2))
			assert.Equal(t, sessionless(halted(t, boot)), sessionless(got), "the preview halts as the boot does")
		})
	}
}

func TestInspect_ACancelledContextIsNotAHalt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waiting := newStep(1, "a", PreOpen, 0, 0)
	waiting.Plan = func(ctx context.Context, _ Env) (Plan, error) {
		return Plan{}, ctx.Err()
	}
	b := newBench(t, waiting)

	_, err := Inspect(ctx, b.home, b.cat, Options{})

	require.ErrorIs(t, err, context.Canceled)
	var h *Halt
	assert.NotErrorAs(t, err, &h)
}

// bootBackups is how many state backups a real boot of the bench takes.
func (b *bench) bootBackups(
	impls Impls,
) int {
	b.tb.Helper()
	before := len(ofKind(b.records(), kindBackup))
	s := b.begin()
	require.NoError(b.tb, s.RunPreOpen(context.Background(), impls))
	require.NoError(b.tb, s.RunPostOpen(context.Background(), impls))
	return len(ofKind(b.records(), kindBackup)) - before
}

func TestInspect_BackupLine(t *testing.T) {
	rewrites := Plan{Actions: []Action{{Kind: Convert, Subject: "state/x", Count: Unknown}}, RewritesState: true}
	neverProbed := func(string) (uint64, error) {
		panic("free space was probed although no copy is due")
	}

	t.Run("a step that rewrites state asks for a backup the size of state", func(t *testing.T) {
		b := newBench(t, planStep(1, "a", PreOpen, 0, 0, rewrites), newStep(2, "b", PostOpen, 0, 0))
		testutil.WriteFile(t, b.home, "state/events/a.db", "123")
		testutil.WriteFile(t, b.home, "state/view.db", "4567")
		testutil.WriteFile(t, b.home, "state/.lock", "not counted")
		var probed string
		b.free = func(path string) (uint64, error) {
			probed = path
			return plentyOfDisk(path)
		}

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, BackupPlan{Required: true, Bytes: 7, FreeBytes: 1 << 40}, report.Backup)
		assert.Equal(t, b.home.Root(), probed)
		assert.Equal(t, 1, b.bootBackups(healthy(1, 2)))
	})

	t.Run("a plan that leaves state alone needs none and probes nothing", func(t *testing.T) {
		b := newBench(t, planStep(1, "a", PreOpen, 0, 0, Plan{Actions: []Action{{Kind: Delete, Subject: "x"}}}))
		testutil.WriteFile(t, b.home, "state/x.db", "bytes")
		b.free = neverProbed

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, BackupPlan{}, report.Backup)
		assert.Zero(t, b.bootBackups(healthy(1)))
	})

	t.Run("a state with no files has nothing to copy", func(t *testing.T) {
		b := newBench(t, planStep(1, "a", PreOpen, 0, 0, rewrites))
		require.NoError(t, os.MkdirAll(filepath.Join(b.home.State(), "empty"), 0o750))
		testutil.WriteFile(t, b.home, "state/.lock", "")
		b.free = neverProbed

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, BackupPlan{}, report.Backup)
		assert.Zero(t, b.bootBackups(healthy(1)))
	})

	t.Run("files of no bytes are still backed up", func(t *testing.T) {
		b := newBench(t, planStep(1, "a", PreOpen, 0, 0, rewrites))
		testutil.WriteFile(t, b.home, "state/upgrades/marker", "")

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, BackupPlan{Required: true, FreeBytes: 1 << 40}, report.Backup)
		assert.Equal(t, 1, b.bootBackups(healthy(1)), "the boot takes the backup the preview announced")
	})
}

// After a boot fails, --plan is how a person sees what the retry will do.
func TestInspect_KeepsTheBackupAnEarlierAttemptTook(t *testing.T) {
	rewrites := Plan{Actions: []Action{{Kind: Convert, Subject: "state/x", Count: Unknown}}, RewritesState: true}
	failedBoot := func(t *testing.T) (*bench, string) {
		t.Helper()
		b := newBench(t, planStep(1, "a", PreOpen, 0, 0, rewrites))
		testutil.WriteFile(t, b.home, "state/a.db", "rows")
		s := b.begin()
		boom := Impls{1: impl(func(*Checkpoints) (Counts, error) { return nil, errors.New("boom") })}
		assert.Equal(t, HaltStepFailed, halted(t, s.RunPreOpen(context.Background(), boom)).Reason)
		require.NoError(t, s.Close())
		taken := ofKind(b.records(), kindBackup)
		require.Len(t, taken, 1)
		return b, taken[0].Run
	}

	t.Run("a backup still on disk is kept, so nothing is copied or probed", func(t *testing.T) {
		b, run := failedBoot(t)
		b.free = func(string) (uint64, error) { panic("the retry probes no free space") }

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, BackupPlan{Kept: run}, report.Backup)
		assert.Zero(t, b.bootBackups(healthy(1)), "the retry takes no second copy")
	})

	t.Run("a backup that is gone is taken again", func(t *testing.T) {
		b, _ := failedBoot(t)
		require.NoError(t, os.RemoveAll(filepath.Join(b.home.MigrateDir(), backupsDirName)))

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Equal(t, BackupPlan{Required: true, Bytes: 4, FreeBytes: 1 << 40}, report.Backup)
		assert.Equal(t, 1, b.bootBackups(healthy(1)))
	})

	t.Run("a backup a completed run followed is not kept", func(t *testing.T) {
		b, _ := failedBoot(t)
		require.Equal(t, 0, b.bootBackups(healthy(1)), "the retry completes on the kept backup")

		report, err := b.inspect()

		require.NoError(t, err)
		assert.Empty(t, report.Steps)
		assert.Equal(t, BackupPlan{}, report.Backup)
	})
}

func TestInspect_ReportsTheLastRun(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	b.seedLedger(
		record{Kind: kindRunStart, At: minute(0), Run: "r1", Version: "v1", From: intPtr(0), Pending: []int{1}},
		record{Kind: kindStepDone, At: minute(1), Run: "r1", Step: 1, Name: "a", Outcome: outcomeApplied, Counts: Counts{"n": 3}},
		record{Kind: kindRunEnd, At: minute(2), Run: "r1", To: intPtr(0), Outcome: outcomeCompleted},
	)

	report, err := b.inspect()

	require.NoError(t, err)
	require.NotNil(t, report.Last)
	assert.Equal(t, RunSummary{
		ID: "r1", Version: "v1", Outcome: outcomeCompleted, Start: minute(0), End: minute(2), Counts: Counts{"n": 3},
	}, *report.Last)
	assert.Empty(t, report.Steps, "the step is done")
}
