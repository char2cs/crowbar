package migrate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBegin_ARecurringStepWhosePlanFailsHaltsTheBoot(t *testing.T) {
	ctx := context.Background()
	fail := false
	step := newStep(1, "sweep", PreOpen, 0, 0)
	step.Recurs = true
	step.Plan = func(context.Context, Env) (Plan, error) {
		if fail {
			return Plan{}, errors.New("cannot look")
		}
		return Plan{}, nil
	}
	b := newBench(t, step)
	first := b.begin()
	require.NoError(t, first.RunPreOpen(ctx, healthy(1)))
	require.NoError(t, first.Close())
	fail = true
	before := b.records()

	h := b.begin().Halt()

	require.NotNil(t, h)
	assert.Equal(t, HaltPlanFailed, h.Reason)
	assert.Equal(t, &ID{Number: 1, Name: "sweep"}, h.Step)
	assert.Contains(t, h.Message, "cannot look")
	assert.Equal(t, before, b.records(), "the halt writes nothing")
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
