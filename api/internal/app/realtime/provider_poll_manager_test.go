package realtime

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
//   - cycles receives once per COMPLETED cycle (the immediate-on-Acquire poll
//     included), which is the signal a negative assertion needs: "the cycle ran to
//     completion, and it polled nothing".
type pollDriver struct {
	ticks  chan time.Time
	cycles chan struct{}
}

func drivePolls(
	m *ProviderPollManager,
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

func TestProviderPollManager_Acquire_StartsPoll(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	d := drivePolls(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	assert.Equal(t, "w1", <-p.calls, "Acquire must poll the workspace")
	<-d.cycles

	// And the cadence keeps polling: one tick, one more poll.
	d.tick()
	assert.Equal(t, "w1", <-p.calls, "a cadence tick must poll again")
}

func TestProviderPollManager_Acquire_PollsImmediately(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	// The tick source is never fired below, so the poll observed here CANNOT have
	// come from the cadence — it is the immediate-on-Acquire poll. Without it a
	// freshly viewed workspace waits a full interval before its PR status (and
	// icon) updates.
	d := drivePolls(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	assert.Equal(t, "w1", <-p.calls)
	<-d.cycles
}

func TestProviderPollManager_Release_StopsPoll(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	d := drivePolls(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	<-p.calls  // polling started
	<-d.cycles // ... and that cycle finished

	m.Release("w1")
	m.waitRunnersForTest() // the run goroutine has RETURNED — not "probably has"

	// Nothing can poll any more: the only runner is gone and no tick was fired.
	assert.Empty(t, p.calls, "poll fired after Release stopped the workspace")
}

func TestProviderPollManager_Refcount(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	d := drivePolls(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	m.Acquire("w1")
	<-p.calls
	<-d.cycles

	m.Release("w1") // refs 2 -> 1, still polling

	// The tick is a rendezvous: it can only be taken by a LIVE run goroutine, so
	// completing it proves the poll outlived the release.
	d.tick()
	assert.Equal(t, "w1", <-p.calls, "poll stopped despite an outstanding subscriber")
}

func TestProviderPollManager_BlankWsID_NoOp(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	drivePolls(m)
	t.Cleanup(m.StopAll)

	m.Acquire("")
	require.NotPanics(t, func() { m.Release("") })
	// Releasing a workspace that was never acquired is a safe no-op.
	require.NotPanics(t, func() { m.Release("never-acquired") })

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	// No runner was ever started, so no poll can ever be issued — this needs no
	// waiting at all, let alone a sleep.
	m.waitRunnersForTest()
	assert.Empty(t, p.calls, "blank wsID started a poll")
}

func TestProviderPollManager_StopAll_Idempotent(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	d := drivePolls(m)

	m.Acquire("w1")
	<-p.calls
	<-d.cycles

	require.NotPanics(t, m.StopAll)
	require.NotPanics(t, m.StopAll) // second call is safe

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

// StopAll returns only once a poll already in flight has finished: shutdown
// closes the stores the poll writes right after it.
func TestProviderPollManager_StopAll_WaitsForAPollInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newFakePoller()
		p.hold = make(chan struct{})
		m := NewProviderPollManager(context.Background(), testPollInterval, p)
		m.Acquire("w1")
		<-p.calls

		assert.False(t, stopReturnedEarly(m.StopAll, p.hold),
			"StopAll returned while the open-time poll was still running")
	})
}

func TestProviderPollManager_AcquireAfterClose_NoOp(t *testing.T) {
	p := newFakePoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, p)
	drivePolls(m)

	m.StopAll()
	m.Acquire("w1")

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	m.waitRunnersForTest()
	assert.Empty(t, p.calls, "Acquire after StopAll started a poll")
}

// TestProviderPollManager_TickSource_DefaultsToIntervalTicker pins the production
// wiring the seam replaces: with no test source installed, run's cadence channel
// is a real interval ticker. It asserts the SOURCE, never that it fires — so
// there is nothing to wait for.
func TestProviderPollManager_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewProviderPollManager(context.Background(), testPollInterval, newFakePoller())

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

// TestProviderPollManager_PollTick_CancelsHungPoll has a timeout as its SUBJECT:
// the injected 10 ms deadline stands in for pollTimeout and is never used as
// synchronisation; every wait below blocks on a real signal.
func TestProviderPollManager_PollTick_CancelsHungPoll(t *testing.T) {
	b := newBlockingPoller()
	m := NewProviderPollManager(context.Background(), testPollInterval, b)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.pollTickWithTimeout(context.Background(), "w1", 10*time.Millisecond)
	}()

	assert.ErrorIs(t, <-b.released, context.DeadlineExceeded,
		"hung poll must be cancelled by the per-poll timeout")
	<-done // pollTick returned once the poll was cancelled
}

// Release and StopAll cancel a hung poll through its context instead of
// waiting out pollTimeout, so StopAll's join is prompt.
func TestProviderPollManager_CancelReachesAHungPoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBlockingPoller()
		m := NewProviderPollManager(context.Background(), testPollInterval, b)
		t.Cleanup(m.StopAll)
		m.Acquire("w1")
		m.Acquire("w2")
		synctest.Wait()

		m.Release("w1")
		require.ErrorIs(t, <-b.released, context.Canceled, "Release must cancel its poll")
		m.StopAll()
		require.ErrorIs(t, <-b.released, context.Canceled, "StopAll must cancel the poll it joins")
	})
}

// ensure fakePoller satisfies the ProviderPoller interface at compile time.
var _ ProviderPoller = (*fakePoller)(nil)
