package termwait_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestDetector_Run_SweepsImmediatelyAndThenOnTheClock(t *testing.T) {
	r := newRigEvery(t, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan domain.AgentTerminalWait, 4)
	r.detector.Run(ctx, func(_, _ string, wait domain.AgentTerminalWait) {
		select {
		case got <- wait:
		default:
		}
	})

	select {
	case wait := <-got:
		assert.True(t, wait.Waiting)
	case <-time.After(5 * time.Second):
		t.Fatal("the first sweep must run immediately, not one interval in")
	}

	r.screens.set(session, idleScreen)
	select {
	case wait := <-got:
		assert.False(t, wait.Waiting)
	case <-time.After(5 * time.Second):
		t.Fatal("the cadence stopped after its first pass")
	}
}

func TestDetector_Run_StopsWithItsContext(t *testing.T) {
	r := newRigEvery(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	swept := make(chan struct{}, 1)
	r.detector.Run(ctx, func(string, string, domain.AgentTerminalWait) {
		swept <- struct{}{}
	})
	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("the immediate sweep must run even on a context that is already done")
	}

	assert.Equal(t, 1, r.runners.callCount(), "the loop must not tick again after its context is done")
}

func TestDetector_Run_DefaultsItsInterval(t *testing.T) {
	r := newRigEvery(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	swept := make(chan struct{}, 1)
	r.detector.Run(ctx, func(string, string, domain.AgentTerminalWait) {
		swept <- struct{}{}
	})

	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("a detector with no declared interval must still sweep")
	}
	assert.Equal(t, termwait.DefaultInterval, 2*time.Second)
}

// §6a: with no live runner the sweep parks after one pass; a runner recorded live wakes it.
func TestDetector_Run_ParksWhileNoRunnerIsLiveAndWakesOnDemand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newRigEvery(t, time.Millisecond)
		r.runners.mu.Lock()
		live := r.runners.live
		r.runners.live = nil
		r.runners.mu.Unlock()

		r.detector.Run(t.Context(), nil)
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, 1, r.runners.callCount(), "one pass, then the loop parks rather than ticks")

		r.runners.mu.Lock()
		r.runners.live = live
		r.runners.mu.Unlock()
		r.detector.Wake()
		time.Sleep(5 * time.Millisecond)
		synctest.Wait()
		assert.Greater(t, r.runners.callCount(), 3, "a live runner resumes the cadence")
	})
}
