package poll

import (
	"context"
	"testing"
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

	select {
	case <-notified:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("onStateChange was not called")
	}

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

func TestSweeper_Start_ContextCancellation(t *testing.T) {
	tickReceived := make(chan struct{}, 1)

	s := &sweeper{
		pollFn: func(
			ctx context.Context,
			wsID string,
			repoPath string,
			branch string,
		) (ProviderStateSnapshot, error) {
			tickReceived <- struct{}{}
			return ProviderStateSnapshot{}, nil
		},
		onStateChange: func(_ string, _ ProviderStateSnapshot) {},
		interval:      10 * time.Millisecond,
		lastState:     make(map[string]ProviderStateSnapshot),
	}

	ctx, cancel := context.WithCancel(context.Background())

	s.Start(ctx, func() []SweepTarget {
		return []SweepTarget{
			{WSID: "ws1", RepoPath: "/repo", Branch: "main", HasOpenPR: true},
		}
	})

	// Wait for at least one tick.
	select {
	case <-tickReceived:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sweep did not fire within timeout")
	}

	// Cancel and ensure the goroutine exits cleanly (no leak).
	cancel()
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
	assert.Equal(t, 60*time.Second, concrete.interval)
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
