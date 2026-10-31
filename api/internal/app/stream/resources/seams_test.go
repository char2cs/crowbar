package resources

import (
	"testing/synctest"
	"time"
)

// Test seams for the background managers: they live here, not in the
// production files, so nothing but tests can reach them.

// driveCyclesForTest installs the deterministic test seams; it must be called
// before Start. ticks replaces the interval ticker (so no cycle ever happens
// unless the test fires one) and cycleDone receives one value after every
// completed cycle, the immediate-on-Start sync included.
func (m *OriginSync) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

// waitRunnersForTest blocks until every run goroutine started by Start has
// actually returned. Stop only cancels a context; this is the real signal
// that the sync has stopped, so a test can assert "nothing fires after
// Stop" without a sleep.
func (m *OriginSync) waitRunnersForTest() {
	m.runners.Wait()
}

// driveCyclesForTest installs the deterministic test seams; it must be called
// before Start. ticks replaces the interval ticker (so no cycle ever happens
// unless the test fires one) and cycleDone receives one value after every
// completed cycle, the immediate-on-Start poll included.
func (m *ForgePoll) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

// waitRunnersForTest blocks until every run goroutine started by Start has
// actually returned. Stop only cancels a context; this is the real
// signal that the poll has stopped, so a test can assert "nothing fires after
// Stop" without a sleep.
func (m *ForgePoll) waitRunnersForTest() {
	m.runners.Wait()
}

// stopReturnedEarly runs stop inside a synctest bubble and reports whether it
// returned while work was still blocked on tail; it then releases tail and
// waits for stop, so nothing outlives the bubble.
func stopReturnedEarly(
	stop func(),
	tail chan struct{},
) bool {
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	synctest.Wait()
	early := false
	select {
	case <-stopped:
		early = true
	default:
	}
	close(tail)
	<-stopped
	return early
}
