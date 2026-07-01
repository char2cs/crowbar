package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePoller records every PollWorkspace call on a channel so tests can
// synchronise on the per-connection ticker without sleeping.
type fakePoller struct {
	calls chan string
}

func newFakePoller() *fakePoller {
	return &fakePoller{calls: make(chan string, 64)}
}

func (f *fakePoller) PollWorkspace(
	_ context.Context,
	wsID string,
) error {
	f.calls <- wsID
	return nil
}

const testPollInterval = time.Millisecond

func TestProviderPollManager_Acquire_StartsPoll(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	select {
	case got := <-p.calls:
		assert.Equal(t, "w1", got)
	case <-time.After(time.Second):
		t.Fatal("poll was not invoked after Acquire")
	}
}

func TestProviderPollManager_Acquire_PollsImmediately(t *testing.T) {
	p := newFakePoller()
	// A long interval ensures the ticker cannot fire within the test window, so
	// observing a poll proves the immediate-on-Acquire poll, not a ticker tick.
	// Without it a freshly viewed workspace waits a full interval before its PR
	// status (and icon) updates.
	m := NewProviderPollManager(context.Background(), time.Hour, p)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	select {
	case got := <-p.calls:
		assert.Equal(t, "w1", got)
	case <-time.After(time.Second):
		t.Fatal("Acquire did not poll immediately (waited for the interval)")
	}
}

func TestProviderPollManager_Release_StopsPoll(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	<-p.calls // confirm polling started
	m.Release("w1")

	// Drain any in-flight tick already buffered at release time.
	drainCalls(p.calls)

	select {
	case <-p.calls:
		t.Fatal("poll fired after Release stopped the workspace")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestProviderPollManager_Refcount(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	m.Acquire("w1")
	<-p.calls
	m.Release("w1") // refs 2 -> 1, still polling

	drainCalls(p.calls)

	select {
	case got := <-p.calls:
		assert.Equal(t, "w1", got)
	case <-time.After(time.Second):
		t.Fatal("poll stopped despite an outstanding subscriber")
	}
}

func TestProviderPollManager_BlankWsID_NoOp(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	t.Cleanup(m.StopAll)

	m.Acquire("")
	require.NotPanics(t, func() { m.Release("") })
	// Releasing a workspace that was never acquired is a safe no-op.
	require.NotPanics(t, func() { m.Release("never-acquired") })

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	select {
	case <-p.calls:
		t.Fatal("blank wsID started a poll")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestProviderPollManager_StopAll_Idempotent(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)

	m.Acquire("w1")
	<-p.calls

	require.NotPanics(t, m.StopAll)
	require.NotPanics(t, m.StopAll) // second call is safe

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestProviderPollManager_AcquireAfterClose_NoOp(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)

	m.StopAll()
	m.Acquire("w1")

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	select {
	case <-p.calls:
		t.Fatal("Acquire after StopAll started a poll")
	case <-time.After(20 * time.Millisecond):
	}
}

// blockingPoller blocks each PollWorkspace on the per-poll context until that
// context is cancelled, then reports the cancellation cause on a channel. It
// proves the manager bounds a hung poll: a remote that never returns must not
// wedge the run goroutine forever.
type blockingPoller struct {
	released chan error
}

func newBlockingPoller() *blockingPoller {
	return &blockingPoller{released: make(chan error, 8)}
}

func (b *blockingPoller) PollWorkspace(
	ctx context.Context,
	_ string,
) error {
	<-ctx.Done()
	b.released <- ctx.Err()
	return ctx.Err()
}

func TestProviderPollManager_PollTick_CancelsHungPoll(t *testing.T) {
	b := newBlockingPoller()
	m := NewProviderPollManager(context.Background(), time.Hour, b)

	// Drive a single tick directly with a short injected timeout so the test does
	// not wait the production 30s. The poll blocks on ctx.Done(); the timeout must
	// fire and unblock it, proving a hung remote cannot wedge the run goroutine.
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		m.pollTickWithTimeout(ctx, "w1", 10*time.Millisecond)
		close(done)
	}()

	select {
	case err := <-b.released:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"hung poll must be cancelled by the per-poll timeout")
	case <-time.After(time.Second):
		t.Fatal("poll was never cancelled — the per-poll timeout did not fire")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pollTick did not return after the poll was cancelled")
	}
}

func TestProviderPollManager_PollTick_DecoupledFromConnCtx(t *testing.T) {
	b := newBlockingPoller()
	m := NewProviderPollManager(context.Background(), time.Hour, b)

	// The per-connection ctx is ALREADY cancelled, yet WithoutCancel must let the
	// poll start (it only stops via its own timeout). This preserves the in-flight
	// Asynx write guarantee while still bounding the poll.
	connCtx, cancelConn := context.WithCancel(context.Background())
	cancelConn()

	go m.pollTickWithTimeout(connCtx, "w1", 10*time.Millisecond)

	select {
	case err := <-b.released:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"poll must run under WithoutCancel and stop only on its own timeout")
	case <-time.After(time.Second):
		t.Fatal("poll did not start/cancel despite an already-cancelled conn ctx")
	}
}

// drainCalls empties any already-buffered calls so a subsequent receive
// observes only post-drain activity.
func drainCalls(
	c chan string,
) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// ensure fakePoller satisfies the ProviderPoller interface at compile time.
var _ ProviderPoller = (*fakePoller)(nil)
