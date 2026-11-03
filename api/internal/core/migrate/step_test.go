package migrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestPhase_String(t *testing.T) {
	assert.Equal(t, "pre-open", PreOpen.String())
	assert.Equal(t, "post-open", PostOpen.String())
	assert.Equal(t, "phase(0)", Phase(0).String())
}

func TestCounts_AddSums(t *testing.T) {
	c := Counts{"moved": 2, "kept": 1}

	c.Add(Counts{"moved": 3, "deleted": 4})

	assert.Equal(t, Counts{"moved": 5, "kept": 1, "deleted": 4}, c)
}

func TestStep_IDCarriesNumberAndName(t *testing.T) {
	s := Step{Number: 7, Name: "chat-types"}

	assert.Equal(t, ID{Number: 7, Name: "chat-types"}, s.ID())
}

// A recurring step, once applied, runs again at each boot whose plan finds
// work, seeing its earlier checkpoints and notes; a boot that finds none writes nothing.
func TestRun_ARecurringStepRunsAgainWhileItsPlanFindsWork(t *testing.T) {
	ctx := context.Background()
	work := true
	var seen []Env
	step := newStep(1, "sweep", PreOpen, 0, 0)
	step.Recurs = true
	step.Plan = func(_ context.Context, env Env) (Plan, error) {
		seen = append(seen, env)
		if !work {
			return Plan{}, nil
		}
		return Plan{Actions: []Action{{Kind: Convert, Subject: "x", Count: 1}}}, nil
	}
	b := newBench(t, step)
	var runs int
	impls := Impls{1: impl(func(cp *Checkpoints) (Counts, error) {
		runs++
		key := fmt.Sprintf("k%d", runs)
		return nil, errors.Join(cp.Mark(key, Counts{"n": 1}), cp.Note("note", Note{Detail: key}))
	})}
	var before []record
	for _, finds := range []bool{true, true, false} {
		work = finds
		before = b.records()
		s := b.begin()
		require.NoError(t, s.RunPreOpen(ctx, impls))
		require.NoError(t, s.Close())
	}

	assert.Equal(t, 2, runs)
	assert.Equal(t, before, b.records(), "a boot whose plan is empty starts no run")
	last := seen[len(seen)-1]
	assert.Equal(t, Marks{"k1": {"n": 1}, "k2": {"n": 1}}, last.Earlier)
	assert.Equal(t, map[string]Note{"note": {Detail: "k2"}}, last.Notes, "the latest note under a key is the one read back")
}

// recurringBench is a home whose recurring step 1 was applied once; plan and
// apply stand in for what that step does on every later boot.
func recurringBench(t *testing.T, plan, apply func() error) (*bench, *bytes.Buffer, Impls) {
	t.Helper()
	step := newStep(1, "sweep", PreOpen, 0, 0)
	step.Recurs = true
	first := true
	step.Plan = func(context.Context, Env) (Plan, error) {
		if first {
			return Plan{Actions: []Action{{Kind: Convert, Subject: "x", Count: 1}}}, nil
		}
		return Plan{Actions: []Action{{Kind: Convert, Subject: "x", Count: 1}}}, plan()
	}
	b := newBench(t, step)
	var logs bytes.Buffer
	b.log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := b.begin()
	require.NoError(t, s.RunPreOpen(context.Background(), healthy(1)))
	require.NoError(t, s.Close())
	first = false
	return b, &logs, Impls{1: impl(func(*Checkpoints) (Counts, error) { return nil, apply() })}
}

// A step that was applied and only runs again for what was added since must
// not stop a boot that already runs on what it did: a plan it cannot make is
// a warning, and the boot writes nothing.
func TestBegin_ARecurringStepWhosePlanFailsIsSkippedWithAWarning(t *testing.T) {
	b, logs, impls := recurringBench(t, func() error { return errors.New("cannot look") }, func() error { return nil })
	before := b.records()

	s := b.begin()

	assert.Nil(t, s.Halt())
	require.NoError(t, s.RunPreOpen(context.Background(), impls))
	assert.Equal(t, before, b.records(), "a skipped step starts no run")
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "cannot look")
}

// A recurring step that fails once applied is skipped: the failure is in the
// ledger as the step's and as a note it reads back, and the run completes.
func TestRun_ARecurringStepThatFailsAgainIsSkippedNotAHalt(t *testing.T) {
	b, logs, impls := recurringBench(t, func() error { return nil }, func() error { return errors.New("legacy file damaged") })

	err := b.begin().RunPreOpen(context.Background(), impls)

	require.NoError(t, err)
	recs := b.records()
	require.Len(t, ofKind(recs, kindStepFail), 1)
	assert.Contains(t, ofKind(recs, kindStepFail)[0].Error, "legacy file damaged")
	notes := ofKind(recs, kindNote)
	require.Len(t, notes, 1)
	assert.Equal(t, skippedNote, notes[0].Key)
	assert.Contains(t, notes[0].Detail, "legacy file damaged")
	assert.Equal(t, outcomeCompleted, ofKind(recs, kindRunEnd)[1].Outcome)
	assert.Contains(t, logs.String(), "level=WARN")
}

// A recurring step whose own check fails once applied is skipped as a failed
// attempt is: rows normal use added since must not halt a home that runs.
func TestRun_ARecurringStepWhoseCheckFailsAgainIsSkippedNotAHalt(t *testing.T) {
	b, logs, impls := recurringBench(t, func() error { return nil }, func() error { return nil })
	impls[1] = Impl{Apply: impls[1].Apply, Verify: func(context.Context, Env) error { return errors.New("holds 402 rows") }}

	require.NoError(t, b.begin().RunPreOpen(context.Background(), impls))

	recs := b.records()
	require.Len(t, ofKind(recs, kindStepFail), 1)
	assert.Contains(t, ofKind(recs, kindNote)[0].Detail, "holds 402 rows")
	assert.Equal(t, outcomeCompleted, ofKind(recs, kindRunEnd)[1].Outcome)
	assert.Contains(t, logs.String(), "level=WARN")
}

func TestRun_ANonRecurringStepIgnoresItsPlanOnceApplied(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	var order []int
	for range 2 {
		s := b.begin()
		require.NoError(t, s.RunPreOpen(ctx, tracing(&order, 1)))
		require.NoError(t, s.Close())
	}

	assert.Equal(t, []int{1}, order, "stubPlan has work, and still the step ran once")
}

// A run killed while a recurring step ran again is closed as interrupted, not
// completed, and the next boot redoes it from the backup that run took, or
// takes the one it never got to.
func TestBegin_ARunKilledWhileARecurringStepRanAgainIsInterrupted(t *testing.T) {
	for _, kill := range []string{kindRunStart, kindBackup, kindCheckpoint} {
		t.Run("after "+kill, func(t *testing.T) {
			step := planned(newStep(1, "sweep", PreOpen, 0, 0), Plan{RewritesState: true, Actions: []Action{{Kind: Convert, Subject: "x", Count: 1}}})
			step.Recurs = true
			b := newBench(t, step)
			testutil.WriteFile(t, b.home, "state/view.db", "rows")
			mark := Impls{1: impl(func(cp *Checkpoints) (Counts, error) { return nil, cp.Mark("chat", Counts{"rows": 1}) })}
			for range 2 {
				s := b.begin()
				require.NoError(t, s.RunPreOpen(context.Background(), mark))
				require.NoError(t, s.Close())
			}
			recs := b.records()
			second := ofKind(recs, kindRunStart)[1].Run
			cut := slices.IndexFunc(recs, func(r record) bool { return r.Run == second && r.Kind == kill }) + 1
			require.Positive(t, cut)
			require.NoError(t, os.Remove(filepath.Join(b.home.MigrateDir(), ledgerName)))
			b.seedLedger(recs[:cut]...)

			var order []int
			s := b.begin()
			require.NoError(t, s.RunPreOpen(context.Background(), tracing(&order, 1)))

			after := b.records()
			ends := ofKind(after, kindRunEnd)
			require.Len(t, ends, 3)
			assert.Equal(t, second, ends[1].Run)
			assert.Equal(t, outcomeInterrupted, ends[1].Outcome)
			assert.Equal(t, []int{1}, order, "the next boot runs the step again")
			assert.Len(t, ofKind(after, kindBackup), 2, "one backup for the first run, one for the attempt that was killed")
		})
	}
}
