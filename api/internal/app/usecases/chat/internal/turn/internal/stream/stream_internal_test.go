package stream

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAwaitOpen_WakesTheInstantARacingObserveArrives is white-box on purpose:
// proving the wake fires the moment a message starts — not merely that it
// eventually times out into the right answer — needs to see AwaitOpen's own
// waiter actually registered before the racing Observe runs, and only this
// package can observe that without a sleep standing in for a real signal.
func TestAwaitOpen_WakesTheInstantARacingObserveArrives(t *testing.T) {
	s := New()
	key := "c" + "\x00" + "r"

	result := make(chan []Message, 1)
	go func() { result <- s.AwaitOpen("c", "r", 5*time.Second) }()

	for {
		s.mu.Lock()
		registered := len(s.waiters[key])
		s.mu.Unlock()
		if registered > 0 {
			break
		}
		runtime.Gosched()
	}

	s.Observe("c", "r", "t", "m", 0, true, true, "hello", time.Now())

	select {
	case open := <-result:
		require.Len(t, open, 1)
		require.Equal(t, "hello", open[0].Text)
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitOpen did not wake when the racing message started")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	require.Empty(t, s.waiters[key], "the woken waiter must be cleared, not left to accumulate")
}
