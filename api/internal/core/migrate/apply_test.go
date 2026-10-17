package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var runIDShape = regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}Z$`)

func TestRun_InLayoutStepRunsOnceAcrossBoots(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	var order []int
	first := b.begin()
	require.NoError(t, first.RunPreOpen(ctx, tracing(&order, 1)))
	require.NoError(t, first.Close())

	second := b.begin()
	require.NoError(t, second.RunPreOpen(ctx, tracing(&order, 1)))

	assert.Equal(t, []int{1}, order)
	done := ofKind(b.records(), kindStepDone)
	require.Len(t, done, 1)
	assert.Equal(t, outcomeApplied, done[0].Outcome)
}

func TestRun_BumpStepWritesLayoutAndIsNotRerun(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PreOpen, 0, 1))
	var order []int
	impls := tracing(&order, 1, 2)
	layoutInVerify := -1
	bump := impls[2]
	bump.Verify = func(context.Context, Env) error {
		layoutInVerify = b.layout()
		return nil
	}
	impls[2] = bump
	s := b.begin()

	require.NoError(t, s.RunPreOpen(ctx, impls))

	assert.Equal(t, 0, layoutInVerify, "LAYOUT moves only after Verify passed")
	assert.Equal(t, 1, b.layout())
	assert.Len(t, ofKind(b.records(), kindStepDone), 2)
	require.NoError(t, s.Close())

	require.NoError(t, b.begin().RunPreOpen(ctx, impls))
	assert.Equal(t, []int{1, 2}, order, "a second boot runs nothing")
	assert.Equal(t, 1, b.layout())
}

func TestRun_PhasesAndOrder(t *testing.T) {
	ctx := context.Background()
	b := newBench(t,
		newStep(1, "one", PreOpen, 0, 0),
		newStep(2, "two", PreOpen, 0, 1),
		newStep(3, "three", PreOpen, 1, 1),
		newStep(4, "four", PostOpen, 1, 1),
		newStep(5, "five", PostOpen, 1, 1),
	)
	var order []int
	impls := tracing(&order, 1, 2, 3, 4, 5)
	s := b.begin()

	require.NoError(t, s.RunPreOpen(ctx, impls))
	assert.Equal(t, []int{1, 2, 3}, order, "post-open waits for RunPostOpen")
	assert.NotContains(t, b.kinds(), kindRunEnd, "the run stays open for the post-open steps")

	require.NoError(t, s.RunPostOpen(ctx, impls))
	assert.Equal(t, []int{1, 2, 3, 4, 5}, order)
	records := b.records()
	assert.Len(t, ofKind(records, kindRunStart), 1, "one run per boot")
	assert.Equal(t, kindRunEnd, records[len(records)-1].Kind)
}

func TestRun_RecordsTheWholeRun(t *testing.T) {
	ctx := context.Background()
	b := newBench(t,
		newStep(1, "one", PreOpen, 0, 0),
		newStep(2, "two", PreOpen, 0, 1),
		newStep(3, "three", PostOpen, 1, 1),
	)
	impls := healthy(1, 2, 3)
	impls[1] = impl(func(cp *Checkpoints) (Counts, error) {
		return Counts{"extra": 2}, cp.Mark("k", Counts{"n": 1})
	})
	impls[3] = impl(func(*Checkpoints) (Counts, error) {
		return Counts{"post": 5}, nil
	})
	s := b.begin()

	require.NoError(t, s.RunPreOpen(ctx, impls))
	require.NoError(t, s.RunPostOpen(ctx, impls))

	records := b.records()
	assert.Equal(t, []string{
		kindRunStart,
		kindStepStart, kindCheckpoint, kindStepDone,
		kindStepStart, kindStepDone,
		kindStepStart, kindStepDone,
		kindRunEnd,
	}, kindsOf(records))
	start, end := records[0], records[len(records)-1]
	assert.Regexp(t, runIDShape, start.Run)
	assert.True(t, singleSegment(start.Run), "the run id names the backup directory")
	assert.Equal(t, "v-test", start.Version)
	assert.Equal(t, []int{1, 2, 3}, start.Pending)
	assert.Equal(t, 0, layoutOf(start.From))
	for _, r := range records {
		assert.Equal(t, start.Run, r.Run)
	}
	done := ofKind(records, kindStepDone)
	assert.Equal(t, Counts{"n": 1, "extra": 2}, done[0].Counts)
	assert.Equal(t, "pre-open", done[0].Phase)
	assert.Equal(t, "post-open", done[2].Phase)
	assert.Equal(t, outcomeCompleted, end.Outcome)
	assert.Equal(t, 1, layoutOf(end.To))
	assert.Equal(t, Counts{"n": 1, "extra": 2, "post": 5}, end.Counts)
}

func TestRun_ResumesFromCheckpoints(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "work", PreOpen, 0, 0))
	failing := Impls{1: impl(func(cp *Checkpoints) (Counts, error) {
		require.NoError(t, cp.Mark("a", Counts{"files": 1}))
		require.NoError(t, cp.Mark("b", Counts{"files": 2}))
		return nil, errors.New("boom")
	})}
	s := b.begin()
	require.Error(t, s.RunPreOpen(ctx, failing))
	require.NoError(t, s.Close())

	var worked []string
	var sawA, sawB bool
	resumed := Impls{1: impl(func(cp *Checkpoints) (Counts, error) {
		sawA, sawB = cp.Done("a"), cp.Done("b")
		for _, key := range []string{"a", "b", "c"} {
			if cp.Done(key) {
				continue
			}
			worked = append(worked, key)
			require.NoError(t, cp.Mark(key, Counts{"files": 4}))
		}
		return Counts{"skipped": 7}, nil
	})}
	require.NoError(t, b.begin().RunPreOpen(ctx, resumed))

	assert.True(t, sawA)
	assert.True(t, sawB)
	assert.Equal(t, []string{"c"}, worked)
	done := ofKind(b.records(), kindStepDone)
	require.Len(t, done, 1)
	assert.Equal(t, Counts{"files": 7, "skipped": 7}, done[0].Counts, "marks from every attempt plus what Apply returned")
}

func TestRun_FailureHaltsWithTheStepAndKeepsLayout(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PreOpen, 0, 1))
	var order []int
	impls := tracing(&order, 1)
	impls[2] = impl(func(*Checkpoints) (Counts, error) {
		order = append(order, 2)
		return nil, errors.New("boom")
	})
	s := b.begin()

	err := s.RunPreOpen(ctx, impls)

	h := halted(t, err)
	assert.Same(t, h, s.Halt())
	require.EqualError(t, err, h.Message)
	assert.Equal(t, HaltStepFailed, h.Reason)
	require.NotNil(t, h.Step)
	assert.Equal(t, ID{Number: 2, Name: "b"}, *h.Step)
	assert.Equal(t, "pre-open", h.Phase)
	assert.Contains(t, h.Message, "boom")
	assert.Equal(t, 0, h.Layout)
	assert.Equal(t, 1, h.Known)
	assert.Equal(t, filepath.Join(b.home.MigrateDir(), ledgerName), h.Ledger)
	records := b.records()
	assert.Equal(t, []string{
		kindRunStart, kindStepStart, kindStepDone, kindStepStart, kindStepFail, kindRunEnd,
	}, kindsOf(records))
	fail, end := records[4], records[5]
	assert.Equal(t, 2, fail.Step)
	assert.Contains(t, fail.Error, "boom")
	assert.Equal(t, outcomeFailed, end.Outcome)
	assert.Equal(t, h.Run, end.Run)
	assert.NoFileExists(t, b.home.LayoutFile(), "a failed bump leaves LAYOUT alone")
	require.NoError(t, s.Close())

	order = nil
	require.NoError(t, b.begin().RunPreOpen(ctx, tracing(&order, 1, 2)))
	assert.Equal(t, []int{2}, order, "the next boot retries the failed step only")
	assert.Equal(t, 1, b.layout())
}

func TestRun_VerifyFailureHalts(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 1))
	applied := 0
	bump := impl(func(*Checkpoints) (Counts, error) {
		applied++
		return nil, nil
	})
	bump.Verify = func(context.Context, Env) error {
		return errors.New("mismatch")
	}
	s := b.begin()

	err := s.RunPreOpen(context.Background(), Impls{1: bump})

	h := halted(t, err)
	assert.Equal(t, HaltVerifyFailed, h.Reason)
	assert.Contains(t, h.Message, "mismatch")
	assert.Equal(t, 1, applied)
	assert.NoFileExists(t, b.home.LayoutFile(), "Apply alone never bumps the layout")
	assert.Empty(t, ofKind(b.records(), kindStepDone))
	assert.Equal(t, []string{kindRunStart, kindStepStart, kindStepFail, kindRunEnd}, b.kinds())
}

func TestRun_FailedLayoutWriteHalts(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 1))
	bump := healthy(1)[1]
	bump.Verify = func(context.Context, Env) error {
		return os.Mkdir(b.home.LayoutFile(), 0o700) // a directory cannot be replaced by the staged file
	}
	s := b.begin()

	err := s.RunPreOpen(context.Background(), Impls{1: bump})

	h := halted(t, err)
	assert.Equal(t, HaltStepFailed, h.Reason)
	assert.Contains(t, h.Message, "LAYOUT")
	assert.Empty(t, ofKind(b.records(), kindStepDone))
}

func TestRun_MissingImplHalts(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PreOpen, 0, 0))
	var order []int
	s := b.begin()

	err := s.RunPreOpen(ctx, tracing(&order, 1))

	h := halted(t, err)
	assert.Equal(t, HaltNoImpl, h.Reason)
	require.NotNil(t, h.Step)
	assert.Equal(t, 2, h.Step.Number)
	assert.Empty(t, order, "no step runs when any step of the phase cannot")
	assert.NoDirExists(t, b.home.MigrateDir())
	require.NoError(t, s.Close())

	nilApply := b.begin()
	err = nilApply.RunPreOpen(ctx, Impls{1: {}, 2: {}})
	assert.Equal(t, HaltNoImpl, halted(t, err).Reason, "an Impl without Apply is no implementation")
}

func TestRun_AnImplementationNoStepOwnsIsAnError(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	var order []int
	s := b.begin()

	err := s.RunPreOpen(ctx, tracing(&order, 1, 9))

	require.ErrorContains(t, err, "unknown implementation")
	require.ErrorContains(t, err, "9")
	assert.Nil(t, s.Halt(), "a wiring mistake is not a halt")
	assert.Empty(t, order)
}

func TestRun_PostOpenWaitsForPendingPreOpenSteps(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PostOpen, 0, 0))
	var order []int
	s := b.begin()

	err := s.RunPostOpen(context.Background(), tracing(&order, 1, 2))

	require.ErrorContains(t, err, "pre-open")
	assert.Nil(t, s.Halt())
	assert.Empty(t, order)
	assert.NoDirExists(t, b.home.MigrateDir())
}

func TestRun_AfterAHaltEverythingReturnsTheHalt(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PostOpen, 0, 0))
	var order []int
	impls := tracing(&order, 1, 2)
	impls[1] = impl(func(*Checkpoints) (Counts, error) {
		return nil, errors.New("boom")
	})
	s := b.begin()
	first := halted(t, s.RunPreOpen(ctx, impls))

	assert.Same(t, first, halted(t, s.RunPostOpen(ctx, impls)))
	assert.Same(t, first, halted(t, s.RunPreOpen(ctx, impls)))
	assert.Empty(t, order, "no step runs after a halt")
}

func TestRun_FailureStaysHaltedWhenTheLedgerCannotRecordIt(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	s := b.begin()
	closeLedger := impl(func(*Checkpoints) (Counts, error) {
		require.NoError(t, s.led.Close()) // the ledger can no longer be written
		return nil, errors.New("boom")
	})

	err := s.RunPreOpen(context.Background(), Impls{1: closeLedger})

	h := halted(t, err)
	assert.Same(t, h, s.Halt())
	assert.Equal(t, HaltStepFailed, h.Reason)
	assert.Contains(t, h.Message, "boom", "the step's own error stays the cause")
	require.ErrorIs(t, err, os.ErrClosed, "the append failure travels with the halt")
}

func TestRun_AnUnrecordedBumpStillHaltsAndKeepsTheBump(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 1))
	s := b.begin()
	closeLedger := impl(func(*Checkpoints) (Counts, error) {
		return nil, s.led.Close()
	})

	err := s.RunPreOpen(context.Background(), Impls{1: closeLedger})

	h := halted(t, err)
	assert.Equal(t, HaltStepFailed, h.Reason)
	assert.Equal(t, 1, h.Layout, "the bump landed and LAYOUT alone decides it is done")
	assert.Equal(t, 1, b.layout())
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestRun_AnUnopenableLedgerIsAnErrorNotAHalt(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	s := b.begin()
	require.NoError(t, os.WriteFile(b.home.MigrateDir(), []byte("in the way"), 0o600))
	var order []int

	err := s.RunPreOpen(context.Background(), tracing(&order, 1))

	require.ErrorContains(t, err, "open ledger")
	assert.Nil(t, s.Halt(), "no step failed, so there is nothing to halt on")
	assert.Empty(t, order)
}

func TestRun_CancelledContextStopsBetweenStepsWithoutAHalt(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PreOpen, 0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var order []int
	impls := tracing(&order, 1, 2)
	one := impls[1]
	impls[1] = impl(func(cp *Checkpoints) (Counts, error) {
		cancel()
		return one.Apply(ctx, Env{}, cp)
	})
	s := b.begin()

	err := s.RunPreOpen(ctx, impls)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, s.Halt(), "an interruption is not a failure")
	assert.Equal(t, []int{1}, order)
	require.NoError(t, s.Close())

	require.NoError(t, b.begin().RunPreOpen(context.Background(), impls))
	assert.Equal(t, []int{1, 2}, order, "the next boot resumes where the run stopped")
}

func TestRun_ContextCancelledBeforeAnyStepWritesNothing(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var order []int

	err := b.begin().RunPreOpen(ctx, tracing(&order, 1))

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, order)
	assert.NoDirExists(t, b.home.MigrateDir())
}

func TestProgress_ReportsThePhaseAndStep(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PostOpen, 0, 0))
	s := b.begin()
	assert.Equal(t, Progress{}, s.Progress())
	entered, release := make(chan struct{}), make(chan struct{})
	impls := healthy(1)
	impls[1] = impl(func(*Checkpoints) (Counts, error) {
		close(entered)
		<-release
		return nil, nil
	})
	var inPostOpen Progress
	impls[2] = impl(func(*Checkpoints) (Counts, error) {
		inPostOpen = s.Progress()
		return nil, nil
	})
	done := make(chan error, 1)
	go func() {
		done <- s.RunPreOpen(ctx, impls)
	}()

	select {
	case <-entered:
	case err := <-done:
		require.FailNow(t, "RunPreOpen returned before step 1 ran", "%v", err)
	}
	// Another goroutine, as the health handler reads it.
	assert.Equal(t, Progress{Phase: PreOpen, Step: ID{Number: 1, Name: "a"}, Index: 1, Total: 2}, s.Progress())
	close(release)
	require.NoError(t, <-done)

	require.NoError(t, s.RunPostOpen(ctx, impls))
	assert.Equal(t, Progress{Phase: PostOpen, Step: ID{Number: 2, Name: "b"}, Index: 2, Total: 2}, inPostOpen)
}
