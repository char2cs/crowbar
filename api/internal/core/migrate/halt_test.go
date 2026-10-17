package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHalt_FailureHaltsWithTheStepAndKeepsLayout(t *testing.T) {
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

func TestHalt_VerifyFailureHalts(t *testing.T) {
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

func TestHalt_FailedLayoutWriteHalts(t *testing.T) {
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

func TestHalt_MissingImplHalts(t *testing.T) {
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
	assert.Empty(t, h.Ledger, "there is no ledger file to point at yet")
	assert.NoDirExists(t, b.home.MigrateDir())
	require.NoError(t, s.Close())

	nilApply := b.begin()
	err = nilApply.RunPreOpen(ctx, Impls{1: {}, 2: {}})
	assert.Equal(t, HaltNoImpl, halted(t, err).Reason, "an Impl without Apply is no implementation")
}

func TestHalt_EverythingAfterReturnsTheHalt(t *testing.T) {
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

func TestHalt_FailureStaysHaltedWhenTheLedgerCannotRecordIt(t *testing.T) {
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

func TestHalt_AnUnrecordedBumpStillHaltsAndKeepsTheBump(t *testing.T) {
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

func TestHalt_LedgerFailuresAroundTheRun(t *testing.T) {
	tests := []struct {
		name string
		// arm runs after Begin; trip is asked on every clock read and breaks the ledger when true.
		arm  func(*bench)
		trip func(*Session) bool
		want string
	}{
		{
			name: "the ledger cannot be opened",
			arm: func(b *bench) {
				require.NoError(b.tb, os.WriteFile(b.home.MigrateDir(), []byte("in the way"), 0o600))
			},
			want: "open ledger",
		},
		{
			name: "run.start cannot be written",
			trip: func(s *Session) bool { return s.led != nil },
			want: "start run",
		},
		{
			name: "step.start cannot be written",
			trip: func(s *Session) bool { return s.run != nil },
			want: "record start",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, newStep(1, "a", PreOpen, 0, 0), newStep(2, "b", PreOpen, 0, 0))
			var s *Session
			tick := tickingClock()
			b.clock = func() time.Time {
				if s != nil && tc.trip != nil && tc.trip(s) {
					require.NoError(t, s.led.Close())
				}
				return tick()
			}
			s = b.begin()
			if tc.arm != nil {
				tc.arm(b)
			}
			var order []int

			err := s.RunPreOpen(context.Background(), tracing(&order, 1, 2))

			h := halted(t, err)
			assert.Same(t, h, s.Halt(), "the boot halts, so the health endpoint can say why")
			assert.Equal(t, HaltStepFailed, h.Reason)
			require.NotNil(t, h.Step)
			assert.Equal(t, 1, h.Step.Number, "the step that would have run first")
			assert.Contains(t, h.Message, tc.want)
			assert.Empty(t, order)
		})
	}
}

func TestHalt_IsSafeToReadWhileTheRunFails(t *testing.T) {
	ctx := context.Background()
	b := newBench(t, newStep(1, "late", PostOpen, 0, 0))
	s := b.begin()
	entered, release := make(chan struct{}), make(chan struct{})
	impls := Impls{1: impl(func(*Checkpoints) (Counts, error) {
		close(entered)
		<-release
		return nil, errors.New("boom")
	})}
	require.NoError(t, s.RunPreOpen(ctx, impls))
	done := make(chan error, 1)
	go func() {
		done <- s.RunPostOpen(ctx, impls)
	}()
	select {
	case <-entered:
	case err := <-done:
		require.FailNow(t, "RunPostOpen returned before step 1 ran", "%v", err)
	}
	// Nothing orders this read against the run's write: -race decides.
	seen := make(chan *Halt, 1)
	go func() {
		seen <- s.Halt()
	}()
	close(release)

	h := halted(t, <-done)
	if got := <-seen; got != nil {
		assert.Same(t, h, got)
	}
	assert.Same(t, h, s.Halt())
}

func TestHalt_JSONShape(t *testing.T) {
	keys := func(h Halt) []string {
		raw, err := json.Marshal(h)
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(raw, &fields))
		return slices.Sorted(maps.Keys(fields))
	}

	t.Run("every field set", func(t *testing.T) {
		h := Halt{
			Reason: HaltStepFailed, Message: "m", Step: &ID{Number: 2, Name: "b"}, Phase: "pre-open",
			Layout: 1, Known: 3, Run: "r", Ledger: "l", Backup: "b",
		}

		assert.Equal(t, []string{"backup", "known", "layout", "ledger", "message", "phase", "reason", "run", "step"}, keys(h))
		raw, err := json.Marshal(h)
		require.NoError(t, err)
		assert.JSONEq(t, `{
			"reason":"step_failed","message":"m","step":{"number":2,"name":"b"},"phase":"pre-open",
			"layout":1,"known":3,"run":"r","ledger":"l","backup":"b"
		}`, string(raw))
	})

	t.Run("a refusal names no step, run, ledger or backup", func(t *testing.T) {
		h := Halt{Reason: HaltLayoutNewer, Message: "m", Layout: 5, Known: 1}

		assert.Equal(t, []string{"known", "layout", "message", "reason"}, keys(h))
	})
}
