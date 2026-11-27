package resources

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePoller struct {
	calls chan string
	hold  chan struct{}
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

const testPollInterval = time.Hour

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

	d.tick()
	assert.Equal(t, "w1", <-p.calls, "a cadence tick must poll again")
}

func TestForgePoll_Start_PollsImmediately(t *testing.T) {
	p := newFakePoller()
	m := NewForgePoll(context.Background(), testPollInterval, p)
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
	<-p.calls
	<-d.cycles

	m.Stop("w1")
	m.waitRunnersForTest()

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
	require.NotPanics(t, m.Close)

	m.mu.Lock()
	count := len(m.cancels)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

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

func TestForgePoll_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewForgePoll(context.Background(), testPollInterval, newFakePoller())

	tickC, stop := m.tickSource()
	require.NotNil(t, tickC)
	stop()
}

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
	<-done
}

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

var _ ProviderPoller = (*fakePoller)(nil)
