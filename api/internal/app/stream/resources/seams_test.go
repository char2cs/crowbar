package resources

import (
	"testing/synctest"
	"time"
)

func (m *OriginSync) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

func (m *OriginSync) waitRunnersForTest() {
	m.runners.Wait()
}

func (m *ForgePoll) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

func (m *ForgePoll) waitRunnersForTest() {
	m.runners.Wait()
}

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
