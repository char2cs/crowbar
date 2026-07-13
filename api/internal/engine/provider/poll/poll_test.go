package poll

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPollFn returns a PollFn that returns a fixed state.
func mockPollFn(
	state ProviderStateSnapshot,
	err error,
) PollFn {
	return func(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (ProviderStateSnapshot, error) {
		return state, err
	}
}

// newTestSweeper builds a sweeper with a pre-allocated lastState map so that
// struct-literal tests do not need to repeat the initialisation.
func newTestSweeper(
	pollFn PollFn,
	onStateChange OnStateChangeFn,
) *sweeper {
	return &sweeper{
		pollFn:        pollFn,
		onStateChange: onStateChange,
		interval:      time.Second,
		lastState:     make(map[string]ProviderStateSnapshot),
	}
}

func TestSweepOnce_SkipsNoOpenPR(t *testing.T) {
	called := false
	s := newTestSweeper(
		func(
			ctx context.Context,
			wsID string,
			repoPath string,
			branch string,
		) (ProviderStateSnapshot, error) {
			called = true
			return ProviderStateSnapshot{}, nil
		},
		func(_ string, _ ProviderStateSnapshot) {},
	)

	s.sweepOnce(context.Background(), []SweepTarget{
		{WSID: "ws1", RepoPath: "/repo", Branch: "main", HasOpenPR: false},
	})

	assert.False(t, called, "pollFn should not be called for targets without open PRs")
}

func TestSweepOnce_CallsPollAndNotifies(t *testing.T) {
	wantState := ProviderStateSnapshot{
		Protected: true,
		PR: &PRInfoSnapshot{
			Number: 42,
			Status: "open",
		},
	}

	notified := make(chan struct{}, 1)
	var gotWsID string
	var gotState ProviderStateSnapshot

	s := newTestSweeper(
		mockPollFn(wantState, nil),
		func(
			wsID string,
			state ProviderStateSnapshot,
		) {
			gotWsID = wsID
			gotState = state
			notified <- struct{}{}
		},
	)

	s.sweepOnce(context.Background(), []SweepTarget{
		{WSID: "ws42", RepoPath: "/repo", Branch: "feature", HasOpenPR: true},
	})

	// sweepOnce calls onStateChange synchronously, so the token is already in the
	// buffer: this receive is a fact, not a wait. (It used to allow 100ms for a
	// callback that, if it had not happened by now, was never going to.)
	<-notified

	assert.Equal(t, "ws42", gotWsID)
	assert.Equal(t, wantState, gotState)
}

func TestSweepOnce_PollError_Silenced(t *testing.T) {
	notifyCalled := false
	s := newTestSweeper(
		func(
			ctx context.Context,
			wsID string,
			repoPath string,
			branch string,
		) (ProviderStateSnapshot, error) {
			return ProviderStateSnapshot{}, assert.AnError
		},
		func(_ string, _ ProviderStateSnapshot) {
			notifyCalled = true
		},
	)

	s.sweepOnce(context.Background(), []SweepTarget{
		{WSID: "ws1", RepoPath: "/repo", Branch: "main", HasOpenPR: true},
	})

	assert.False(t, notifyCalled, "onStateChange should not be called on poll error")
}

// The sweep loop is driven by a real time.Ticker at the production cadence. The
// test runs inside a synctest bubble, so that ticker fires against the bubble's
// fake clock the moment every goroutine is durably blocked — no interval is
// shortened, no wall-clock duration is waited on, and the *production* 5-minute
// cadence is the thing actually exercised. The bubble also refuses to end while
// a goroutine is still alive, so the cancel-exits-cleanly (no-leak) half of this
// test is enforced by synctest itself rather than by hope.
func TestSweeper_Start_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tickReceived := make(chan struct{}, 8)

		s := &sweeper{
			pollFn: func(
				ctx context.Context,
				wsID string,
				repoPath string,
				branch string,
			) (ProviderStateSnapshot, error) {
				select {
				case tickReceived <- struct{}{}:
				default:
				}
				return ProviderStateSnapshot{}, nil
			},
			onStateChange: func(_ string, _ ProviderStateSnapshot) {},
			interval:      GlobalCronInterval,
			lastState:     make(map[string]ProviderStateSnapshot),
		}

		ctx, cancel := context.WithCancel(context.Background())

		s.Start(ctx, func() []SweepTarget {
			return []SweepTarget{
				{WSID: "ws1", RepoPath: "/repo", Branch: "main", HasOpenPR: true},
			}
		})

		// Blocks until the ticker actually fires and the sweep actually polls.
		<-tickReceived

		// Cancel; synctest will not let the bubble finish until run() has returned.
		cancel()
	})
}

func TestSweepOnce_NoCallbackOnRepeatState(t *testing.T) {
	state := ProviderStateSnapshot{Protected: true}
	callCount := 0
	s := newTestSweeper(
		mockPollFn(state, nil),
		func(_ string, _ ProviderStateSnapshot) {
			callCount++
		},
	)

	target := SweepTarget{WSID: "ws1", RepoPath: "/repo", Branch: "main", HasOpenPR: true}

	// First sweep: state is new → callback fires.
	s.sweepOnce(context.Background(), []SweepTarget{target})
	assert.Equal(t, 1, callCount)

	// Second sweep: state unchanged → callback must NOT fire again.
	s.sweepOnce(context.Background(), []SweepTarget{target})
	assert.Equal(t, 1, callCount, "callback should not fire when state has not changed")
}

func TestNewSweeper_DefaultInterval(t *testing.T) {
	s := NewSweeper(
		mockPollFn(ProviderStateSnapshot{}, nil),
		func(_ string, _ ProviderStateSnapshot) {},
	)
	require.NotNil(t, s)
	concrete, ok := s.(*sweeper)
	require.True(t, ok)
	assert.Equal(t, GlobalCronInterval, concrete.interval)
	assert.Equal(t, 5*time.Minute, concrete.interval)
}

func TestPollIntervalConstants(t *testing.T) {
	assert.Equal(t, 5*time.Minute, GlobalCronInterval)
	assert.Equal(t, 1*time.Minute, PerConnectionInterval)
}

func TestNewSweeperWithInterval(t *testing.T) {
	s := newSweeperWithInterval(
		mockPollFn(ProviderStateSnapshot{}, nil),
		func(_ string, _ ProviderStateSnapshot) {},
		5*time.Millisecond,
	)
	require.NotNil(t, s)
	concrete, ok := s.(*sweeper)
	require.True(t, ok)
	assert.Equal(t, 5*time.Millisecond, concrete.interval)
}

func TestSweepTarget_MultipleTargets(t *testing.T) {
	var polled []string
	notified := make(chan string, 5)

	s := newTestSweeper(
		func(
			ctx context.Context,
			wsID string,
			repoPath string,
			branch string,
		) (ProviderStateSnapshot, error) {
			polled = append(polled, wsID)
			return ProviderStateSnapshot{Protected: true}, nil
		},
		func(
			wsID string,
			_ ProviderStateSnapshot,
		) {
			notified <- wsID
		},
	)

	targets := []SweepTarget{
		{WSID: "ws1", HasOpenPR: true, RepoPath: "/r1", Branch: "b1"},
		{WSID: "ws2", HasOpenPR: false, RepoPath: "/r2", Branch: "b2"}, // skipped
		{WSID: "ws3", HasOpenPR: true, RepoPath: "/r3", Branch: "b3"},
	}

	s.sweepOnce(context.Background(), targets)

	close(notified)
	var notifiedIDs []string
	for id := range notified {
		notifiedIDs = append(notifiedIDs, id)
	}

	assert.Equal(t, []string{"ws1", "ws3"}, polled)
	assert.Equal(t, []string{"ws1", "ws3"}, notifiedIDs)
}

// A pollFn that blocks until its ctx is cancelled mimics a hung remote. The
// per-target timeout must fire so one stuck target cannot stall the whole serial
// sweep — every other workspace's PR status would otherwise stop updating until
// the hang resolved.
//
// The timeout IS the subject here, so the clock cannot be removed — it is moved.
// Inside a synctest bubble the production perTargetTimeout (30s, via the
// timeout<=0 fallback) elapses on the bubble's fake clock as soon as everything
// is durably blocked, so the test exercises the real bound, instantly, with no
// shortened duration to tune and no wall-clock guard to false-fail under load.
func TestSweepTarget_HungPollIsCancelledByPerTargetTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gotErr := make(chan error, 1)
		s := newTestSweeper(
			func(
				ctx context.Context,
				wsID string,
				repoPath string,
				branch string,
			) (ProviderStateSnapshot, error) {
				<-ctx.Done()
				gotErr <- ctx.Err()
				return ProviderStateSnapshot{}, ctx.Err()
			},
			func(_ string, _ ProviderStateSnapshot) {
				t.Error("onStateChange must not fire when the poll was cancelled")
			},
		)

		done := make(chan struct{})
		go func() {
			s.sweepTarget(context.Background(), SweepTarget{
				WSID: "ws1", RepoPath: "/r", Branch: "b", HasOpenPR: true,
			})
			close(done)
		}()

		err := <-gotErr
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"hung target poll must be cancelled by the per-target timeout")

		<-done
	})
}

func TestStatesEqual(t *testing.T) {
	pr := &PRInfoSnapshot{Number: 1, Status: "open", URL: "u", Title: "t", TargetBranch: "main"}

	assert.True(t, statesEqual(ProviderStateSnapshot{}, ProviderStateSnapshot{}))
	assert.True(t, statesEqual(
		ProviderStateSnapshot{Protected: true, PR: pr},
		ProviderStateSnapshot{Protected: true, PR: &PRInfoSnapshot{Number: 1, Status: "open", URL: "u", Title: "t", TargetBranch: "main"}},
	))
	assert.False(t, statesEqual(
		ProviderStateSnapshot{Protected: true},
		ProviderStateSnapshot{Protected: false},
	))
	assert.False(t, statesEqual(
		ProviderStateSnapshot{PR: pr},
		ProviderStateSnapshot{PR: nil},
	))
	assert.False(t, statesEqual(
		ProviderStateSnapshot{PR: pr},
		ProviderStateSnapshot{PR: &PRInfoSnapshot{Number: 2}},
	))
}

// TestSweepOnce_PrunesLastStateForRemovedWorkspaces proves pass-3 #3: a workspace
// that drops out of the sweep targets (deleted) has its stale lastState entry
// pruned, so the map can't grow unboundedly across the daemon's lifetime.
func TestSweepOnce_PrunesLastStateForRemovedWorkspaces(t *testing.T) {
	s := newTestSweeper(
		mockPollFn(ProviderStateSnapshot{
			Protected: true,
			PR:        &PRInfoSnapshot{Number: 1, Status: "open"},
		}, nil),
		func(_ string, _ ProviderStateSnapshot) {},
	)

	a := SweepTarget{WSID: "wsA", RepoPath: "/a", Branch: "main", HasOpenPR: true}
	b := SweepTarget{WSID: "wsB", RepoPath: "/b", Branch: "main", HasOpenPR: true}

	// First sweep records both PR-bearing workspaces.
	s.sweepOnce(context.Background(), []SweepTarget{a, b})
	s.mu.Lock()
	_, hasA := s.lastState["wsA"]
	_, hasB := s.lastState["wsB"]
	s.mu.Unlock()
	require.True(t, hasA)
	require.True(t, hasB)

	// wsB is deleted: the next tick's targets contain only wsA, so wsB's now-stale
	// entry must be pruned rather than lingering forever.
	s.sweepOnce(context.Background(), []SweepTarget{a})
	s.mu.Lock()
	_, hasA = s.lastState["wsA"]
	_, hasB = s.lastState["wsB"]
	n := len(s.lastState)
	s.mu.Unlock()
	assert.True(t, hasA, "an active workspace keeps its lastState entry")
	assert.False(t, hasB, "a removed workspace's stale entry is pruned")
	assert.Equal(t, 1, n)
}
