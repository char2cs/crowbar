package realtime

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// ProviderPoller is the usecase surface the per-connection poll needs. It is
// satisfied by usecases/provider.Usecase so the manager stays decoupled from the
// concrete usecase wiring and is trivially fakeable in tests.
type ProviderPoller interface {
	PollWorkspace(
		ctx context.Context,
		wsID string,
	) error
}

// providerPollHandle tracks one workspace's running per-connection poll: its
// subscriber refcount and the cancel func that stops the ticking goroutine.
type providerPollHandle struct {
	refs   int
	cancel context.CancelFunc
}

// ProviderPollManager is the refcounted, lazy lifecycle registry for the
// per-active-WS-connection provider poll (D10/§11). It mirrors WatcherManager
// with its OWN handle map so its refcount is independent of the watcher/LSP
// lifecycles: the first subscriber on a single workspace WS starts a goroutine
// ticking every interval, and the last unsubscribe stops it. Each tick issues
// the provider poll on a context.WithoutCancel-derived context so a mid-tick
// cancel (Release/StopAll) never aborts an in-flight Asynx write.
type ProviderPollManager struct {
	root     context.Context
	interval time.Duration
	poll     ProviderPoller
	mu       sync.Mutex
	handles  map[string]*providerPollHandle
	closed   bool

	// runners tracks the live run goroutines. Cancelling a context only ASKS a
	// runner to stop; runners.Wait is the real "it has stopped" signal, which
	// tests block on (waitRunnersForTest) instead of sleeping.
	runners sync.WaitGroup

	// ticks and cycleDone are the deterministic test seams installed by
	// driveCyclesForTest; both are nil in production. See tickSource/notifyCycle.
	ticks     <-chan time.Time
	cycleDone chan<- struct{}
}

// pollTimeout bounds a single PollWorkspace invocation. The chain runs network
// subprocesses (gh auth status, gh api, git ls-remote, gh pr list) and a hung
// remote (network partition, credential prompt on stdin) would otherwise leak
// the poll goroutine + child forever and stall all PR polling. WithoutCancel
// keeps the poll decoupled from the per-connection ctx; this timeout still
// guarantees forward progress on every tick.
const pollTimeout = 30 * time.Second

// NewProviderPollManager builds the manager over the per-connection poll
// interval and the provider-poll usecase. root is the parent context for every
// poll goroutine; cancelling it (via the realtime Service) stops all polls.
func NewProviderPollManager(
	root context.Context,
	interval time.Duration,
	poll ProviderPoller,
) *ProviderPollManager {
	return &ProviderPollManager{
		root:     root,
		interval: interval,
		poll:     poll,
		handles:  make(map[string]*providerPollHandle),
	}
}

// Acquire records one subscriber for wsID. On the 0->1 transition it starts the
// poll goroutine under a cancelable context. A blank wsID is ignored so a
// list-scope (no :wsId) subscribe can never start an unscoped poll. After
// StopAll it is a no-op so a late subscribe from a not-yet-closed hijacked WS
// connection cannot start a poll under the cancelled root.
func (m *ProviderPollManager) Acquire(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if h, ok := m.handles[wsID]; ok {
		h.refs++
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.root)
	m.handles[wsID] = &providerPollHandle{refs: 1, cancel: cancel}
	m.runners.Add(1)
	m.mu.Unlock()

	go m.run(ctx, wsID)
}

// Release drops one subscriber for wsID. On the 1->0 transition it cancels the
// poll goroutine. A blank wsID and an unknown wsID are no-ops.
func (m *ProviderPollManager) Release(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	h, ok := m.handles[wsID]
	if !ok {
		m.mu.Unlock()
		return
	}
	h.refs--
	if h.refs > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.handles, wsID)
	m.mu.Unlock()

	h.cancel()
}

// StopAll cancels every live poll and clears the handle map. It is idempotent
// and marks the manager closed so subsequent Acquire calls no-op. It runs on
// graceful shutdown (via the realtime Service Close).
func (m *ProviderPollManager) StopAll() {
	m.mu.Lock()
	m.closed = true
	handles := m.handles
	m.handles = make(map[string]*providerPollHandle)
	m.mu.Unlock()

	for _, h := range handles {
		h.cancel()
	}
}

// run ticks every interval and issues a single-workspace provider poll until the
// per-connection context is cancelled. The poll runs on a WithoutCancel-derived
// context so a Release/StopAll mid-tick cannot abort an in-flight Asynx write.
func (m *ProviderPollManager) run(
	ctx context.Context,
	wsID string,
) {
	defer m.runners.Done()
	defer safego.Recover("realtime.providerPoll.run")
	// Poll once immediately on the 0->1 subscriber transition so a freshly viewed
	// workspace's PR status (and sidebar icon) updates within ~1s instead of after
	// a full interval. Skip it only if the subscriber already released mid-startup.
	select {
	case <-ctx.Done():
		return
	default:
		m.pollTick(ctx, wsID)
		m.notifyCycle(ctx)
	}

	tickC, stopTicker := m.tickSource()
	defer stopTicker()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			m.pollTick(ctx, wsID)
			m.notifyCycle(ctx)
		}
	}
}

// tickSource returns the cadence channel run listens on, plus its stop func. In
// production that is a real interval ticker. A test installs its own source
// (driveCyclesForTest) so a cycle happens exactly when the test fires one and the
// background cadence can never race the test — the same discipline as
// engine/terminal's StopMaintenanceForTest + RunMaintenanceOnceForTest. Sleeping
// for the production poll interval is not an option, and shrinking the interval
// to "small enough" would only trade a slow test for a flaky one.
func (m *ProviderPollManager) tickSource() (<-chan time.Time, func()) {
	m.mu.Lock()
	ticks := m.ticks
	m.mu.Unlock()

	if ticks != nil {
		return ticks, func() {}
	}
	ticker := time.NewTicker(m.interval)
	return ticker.C, ticker.Stop
}

// notifyCycle signals the installed test seam that one full poll cycle has
// finished. It is the REAL end-of-cycle signal a test blocks on to assert that
// NOTHING happened — an assertion that would otherwise need a sleep, which is a
// guess. It is a no-op in production, and it gives up on ctx cancellation so a
// runner can never be wedged by it.
func (m *ProviderPollManager) notifyCycle(
	ctx context.Context,
) {
	m.mu.Lock()
	done := m.cycleDone
	m.mu.Unlock()

	if done == nil {
		return
	}
	select {
	case done <- struct{}{}:
	case <-ctx.Done():
	}
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

// pollTick issues a single PollWorkspace on a context.WithoutCancel-derived,
// timeout-bounded context. WithoutCancel keeps an in-flight Asynx write from
// being aborted by a mid-tick Release/StopAll; the timeout still cancels a poll
// whose subprocesses hang, so the run goroutine cannot wedge forever.
func (m *ProviderPollManager) pollTick(
	ctx context.Context,
	wsID string,
) {
	m.pollTickWithTimeout(ctx, wsID, pollTimeout)
}

// pollTickWithTimeout is pollTick with an injectable timeout (the test seam for
// a short interval). The cancel func runs before this returns so no timer
// outlives the tick.
func (m *ProviderPollManager) pollTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
) {
	pollCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	_ = m.poll.PollWorkspace(pollCtx, wsID)
}
