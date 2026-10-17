package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	want := filepath.Join(b.home.MigrateDir(), backupsDirName, records[0].Run, "state")
	assert.Equal(t, want, h.Backup)
	assert.Equal(t, []string{
		kindRunStart, kindBackup, kindStepStart, kindStepDone, kindStepStart, kindStepFail, kindRunEnd,
	}, kindsOf(records))
	backup := records[1]
	assert.Equal(t, want, backup.Path)
	assert.Equal(t, int64(len("the rows before")), backup.Bytes)
	assert.Equal(t, records[0].Run, backup.Run)
	assert.NoFileExists(t, filepath.Join(want, homeLockName), "the lock file is not state")
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
	raw, err := os.ReadFile(filepath.Join(firstHalt.Backup, "view.db"))
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
	raw, err = os.ReadFile(filepath.Join(backups[1].Path, "view.db"))
	require.NoError(t, err)
	assert.Equal(t, "half migrated", string(raw))
}

func TestRun_ABackupSomeoneRemovedIsTakenAgain(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, planned(newStep(1, "a", PreOpen, 0, 0), Plan{RewritesState: true}))
	testutil.WriteFile(t, b.home, "state/view.db", "rows")
	failing := Impls{1: impl(func(*Checkpoints) (Counts, error) {
		return nil, errors.New("boom")
	})}
	first := b.begin()
	firstHalt := halted(t, first.RunPreOpen(ctx, failing))
	require.NoError(t, first.Close())
	require.NoError(t, os.RemoveAll(filepath.Dir(firstHalt.Backup)))

	secondHalt := halted(t, b.begin().RunPreOpen(ctx, failing))

	assert.NotEqual(t, firstHalt.Backup, secondHalt.Backup, "a backup that is gone is no backup")
	assert.DirExists(t, secondHalt.Backup)
}
