package resources

import (
	"context"
	"errors"
	"log/slog"
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

// ForgePoll polls a held workspace's forge (pull-request state) while a client
// holds its ws: scope: once at Start, then every interval. Each poll runs under
// the runner's context, so Stop and Close cancel it rather than wait it out.
type ForgePoll struct {
	root     context.Context
	interval time.Duration
	poll     ProviderPoller
	mu       sync.Mutex
	cancels  map[string]context.CancelFunc
	closed   bool

	// runners tracks the live run goroutines. Cancelling a context only ASKS a
	// runner to stop; runners.Wait is the real "it has stopped" signal, which
	// Close joins on and tests block on after Stop (waitRunnersForTest).
	runners sync.WaitGroup

	// ticks and cycleDone are the deterministic test seams installed by
	// driveCyclesForTest; both are nil in production. See tickSource/notifyCycle.
	ticks     <-chan time.Time
	cycleDone chan<- struct{}
}

// pollTimeout bounds a single PollWorkspace invocation. The chain runs network
// subprocesses (gh auth status, gh api, git ls-remote, gh pr list) and a hung
// remote (network partition, credential prompt on stdin) would otherwise leak
// the poll goroutine + child forever and stall all PR polling.
const pollTimeout = 30 * time.Second

// NewForgePoll builds the resource over the poll interval and the provider-poll
// usecase. root is the parent context for every poll goroutine.
func NewForgePoll(
	root context.Context,
	interval time.Duration,
	poll ProviderPoller,
) *ForgePoll {
	return &ForgePoll{
		root:     root,
		interval: interval,
		poll:     poll,
		cancels:  make(map[string]context.CancelFunc),
	}
}

// Start begins polling wsID. After Close it refuses, so a late hold cannot
// start a poll under the cancelled root.
func (m *ForgePoll) Start(
	wsID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errClosed
	}
	ctx, cancel := context.WithCancel(m.root)
	m.cancels[wsID] = cancel
	m.runners.Add(1)
	go m.run(ctx, wsID)
	return nil
}

// Stop cancels wsID's poll, a poll in flight included.
func (m *ForgePoll) Stop(
	wsID string,
) {
	m.mu.Lock()
	cancel, ok := m.cancels[wsID]
	delete(m.cancels, wsID)
	m.mu.Unlock()
	if ok {
		cancel()
	}
}

// Close cancels every live poll and joins every runner, so no poll outlives
// shutdown into a closed store. It is idempotent and refuses later starts.
func (m *ForgePoll) Close() {
	m.mu.Lock()
	m.closed = true
	cancels := m.cancels
	m.cancels = make(map[string]context.CancelFunc)
	m.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	m.runners.Wait()
}

// run ticks every interval and issues a single-workspace provider poll until the
// per-connection context is cancelled, which also cancels a poll in flight.
func (m *ForgePoll) run(
	ctx context.Context,
	wsID string,
) {
	defer m.runners.Done()
	defer safego.Recover("resources.forgePoll.run")
	// Poll once immediately on Start so a freshly viewed
	// workspace's PR status (and sidebar icon) updates within ~1s instead of after
	// a full interval. Skip it only if the hold already ended mid-startup.
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
// core/terminal's StopMaintenanceForTest + RunMaintenanceOnceForTest. Sleeping
// for the production poll interval is not an option, and shrinking the interval
// to "small enough" would only trade a slow test for a flaky one.
func (m *ForgePoll) tickSource() (<-chan time.Time, func()) {
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
func (m *ForgePoll) notifyCycle(
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

// pollTick issues a single PollWorkspace bounded by pollTimeout, so a poll whose
// subprocesses hang cannot wedge the run goroutine forever.
func (m *ForgePoll) pollTick(
	ctx context.Context,
	wsID string,
) {
	m.pollTickWithTimeout(ctx, wsID, pollTimeout)
}

// pollTickWithTimeout is pollTick with an injectable timeout (the test seam for
// a short interval). The cancel func runs before this returns so no timer
// outlives the tick.
func (m *ForgePoll) pollTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// A tick has no caller: a failed poll is logged, and the next tick tries again.
	if err := m.poll.PollWorkspace(pollCtx, wsID); err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(ctx, "resources: forge poll", "ws", wsID, "err", err)
	}
}
