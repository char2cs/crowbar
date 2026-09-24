package realtime

import (
	"context"
	"time"
)

// Test seams for the background managers: they live here, not in the
// production files, so nothing but tests can reach them.

// driveCyclesForTest installs the deterministic test seams; it must be called
// before Acquire. ticks replaces the interval ticker (so no cycle ever happens
// unless the test fires one) and cycleDone receives one value after every
// completed cycle, the immediate-on-Acquire sync included.
func (m *OriginSyncManager) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

// waitRunnersForTest blocks until every run goroutine started by Acquire has
// actually returned. Release/StopAll only cancel a context; this is the real
// signal that the sync has stopped, so a test can assert "nothing fires after
// Release" without a sleep.
func (m *OriginSyncManager) waitRunnersForTest() {
	m.runners.Wait()
}

// driveCyclesForTest installs the deterministic test seams; it must be called
// before Acquire. ticks replaces the interval ticker (so no cycle ever happens
// unless the test fires one) and cycleDone receives one value after every
// completed cycle, the immediate-on-Acquire poll included.
func (m *ProviderPollManager) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

// waitRunnersForTest blocks until every run goroutine started by Acquire has
// actually returned. Release/StopAll only cancel a context; this is the real
// signal that the poll has stopped, so a test can assert "nothing fires after
// Release" without a sleep.
func (m *ProviderPollManager) waitRunnersForTest() {
	m.runners.Wait()
}

// NoopLSPLifecycle returns a lifecycle whose edges do no work, for tests that
// have no LSP engine to release.
func NoopLSPLifecycle() LSPLifecycle {
	return noopLSPLifecycle{}
}

type noopLSPLifecycle struct{}

func (noopLSPLifecycle) Ensure(
	_ context.Context,
	_ string,
) {
}

func (noopLSPLifecycle) Shutdown(
	_ context.Context,
	_ string,
) {
}
