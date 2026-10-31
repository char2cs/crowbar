package resources

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePoller records every PollWorkspace call on a channel so tests block on the
// REAL "a poll happened" signal instead of sleeping.
type fakePoller struct {
	calls chan string
	// hold, when set, keeps every PollWorkspace in flight until it is closed.
	hold chan struct{}
}

func newFakePoller() *fakePoller {
	return &fakePoller{calls: make(chan string, 64)}
}

func (f *fakePoller) PollWorkspace(
	_ context.Context,
	wsID string,
) error {
	f.calls <- wsID
	if f.hold != nil {
		<-f.hold
	}
	return nil
}

// testPollInterval is deliberately far longer than any test can run: no test
// waits for the cadence, and no cadence may fire behind a test's back. Cycles are
// driven explicitly through the injected tick source (pollDriver), so the manager
// polls exactly when the test says so — never on a clock.
const testPollInterval = time.Hour

// pollDriver owns the manager's two deterministic seams.
//
//   - ticks is UNBUFFERED: sending on it is a rendezvous with the run goroutine,
//     so the send itself proves a runner is alive and has taken the tick.
//   - cycles receives once per COMPLETED cycle (the immediate-on-Start poll
//     included), which is the signal a negative assertion needs: "the cycle ran to
//     completion, and it polled nothing".
type pollDriver struct {
	ticks  chan time.Time
	cycles chan struct{}
}

func drivePolls(
	m *ForgePoll,
) *pollDriver {
	d := &pollDriver{ticks: make(chan time.Time), cycles: make(chan struct{}, 64)}
	m.driveCyclesForTest(d.ticks, d.cycles)
	return d
}

// tick fires exactly one cycle and returns once that cycle has completed.
func (d *pollDriver) tick() {
	d.ticks <- time.Time{}
	<-d.cycles
}

func TestForgePoll_Start_StartsPoll(t *testing.T) {
	p := newFakePoller()
	m := NewForgePoll(context.Background(), testPollInterval, p)
	d := drivePolls(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w1"))

	assert.Equal(t, "w1", <-p.calls, "Start must poll the workspace")
	<-d.cycles

	// And the cadence keeps polling: one tick, one more poll.
	d.tick()
	assert.Equal(t, "w1", <-p.calls, "a cadence tick must poll again")
}

func TestForgePoll_Start_PollsImmediately(t *testing.T) {
	p := newFakePoller()
	m := NewForgePoll(context.Background(), testPollInterval, p)
	// The tick source is never fired below, so the poll observed here CANNOT have
	// come from the cadence — it is the immediate-on-Start poll. Without it a
	// freshly viewed workspace waits a full interval before its PR status (and
	// icon) updates.
	d := drivePolls(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w1"))

	assert.Equal(t, "w1", <-p.calls)
	<-d.cycles
}

func TestForgePoll_Stop_StopsPoll(t *testing.T) {
	p := newFakePoller()
	m := NewForgePoll(context.Background(), testPollInterval, p)
	d := drivePolls(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w1"))
	<-p.calls  // polling started
	<-d.cycles // ... and that cycle finished

	m.Stop("w1")
	m.waitRunnersForTest() // the run goroutine has RETURNED — not "probably has"

	// Nothing can poll any more: the only runner is gone and no tick was fired.
	assert.Empty(t, p.calls, "poll fired after Stop stopped the workspace")
}

func TestForgePoll_Close_Idempotent(t *testing.T) {
	p := newFakePoller()
	m := NewForgePoll(context.Background(), testPollInterval, p)
	d := drivePolls(m)

	require.NoError(t, m.Start("w1"))
	<-p.calls
	<-d.cycles

	require.NotPanics(t, m.Close)
	require.NotPanics(t, m.Close) // second call is safe

	m.mu.Lock()
	count := len(m.cancels)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

// Close returns only once a poll already in flight has finished: shutdown
// closes the stores the poll writes right after it.
func TestForgePoll_Close_WaitsForAPollInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newFakePoller()
		p.hold = make(chan struct{})
		m := NewForgePoll(context.Background(), testPollInterval, p)
		require.NoError(t, m.Start("w1"))
		<-p.calls

		assert.False(t, stopReturnedEarly(m.Close, p.hold),
			"Close returned while the open-time poll was still running")
	})
}

func TestForgePoll_StartAfterClose_Refused(t *testing.T) {
	p := newFakePoller()
	m := NewForgePoll(context.Background(), testPollInterval, p)
	drivePolls(m)

	m.Close()
	require.ErrorIs(t, m.Start("w1"), errClosed)

	m.mu.Lock()
	count := len(m.cancels)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	m.waitRunnersForTest()
	assert.Empty(t, p.calls, "Start after Close started a poll")
}

// TestForgePoll_TickSource_DefaultsToIntervalTicker pins the production
// wiring the seam replaces: with no test source installed, run's cadence channel
// is a real interval ticker. It asserts the SOURCE, never that it fires — so
// there is nothing to wait for.
func TestForgePoll_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewForgePoll(context.Background(), testPollInterval, newFakePoller())

	tickC, stop := m.tickSource()
	require.NotNil(t, tickC)
	stop()
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

// TestForgePoll_PollTick_CancelsHungPoll has a timeout as its SUBJECT:
// the injected 10 ms deadline stands in for pollTimeout and is never used as
// synchronisation; every wait below blocks on a real signal.
func TestForgePoll_PollTick_CancelsHungPoll(t *testing.T) {
	b := newBlockingPoller()
	m := NewForgePoll(context.Background(), testPollInterval, b)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.pollTickWithTimeout(context.Background(), "w1", 10*time.Millisecond)
	}()

	assert.ErrorIs(t, <-b.released, context.DeadlineExceeded,
		"hung poll must be cancelled by the per-poll timeout")
	<-done // pollTick returned once the poll was cancelled
}

// Stop and Close cancel a hung poll through its context instead of
// waiting out pollTimeout, so Close's join is prompt.
func TestForgePoll_CancelReachesAHungPoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBlockingPoller()
		m := NewForgePoll(context.Background(), testPollInterval, b)
		t.Cleanup(m.Close)
		require.NoError(t, m.Start("w1"))
		require.NoError(t, m.Start("w2"))
		synctest.Wait()

		m.Stop("w1")
		require.ErrorIs(t, <-b.released, context.Canceled, "Stop must cancel its poll")
		m.Close()
		require.ErrorIs(t, <-b.released, context.Canceled, "Close must cancel the poll it joins")
	})
}

// ensure fakePoller satisfies the ProviderPoller interface at compile time.
var _ ProviderPoller = (*fakePoller)(nil)
