package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Shutdown gracefully quiesces the application layer's asynchronous machinery
// within the ctx deadline, BEFORE the adapter closes the DBs (spec §3.8 steps
// 2-3, decision 11). It runs ahead of Close in the ordered teardown, and its whole
// job is to leave every WRITER quiesced before the things they write to go away —
// in dependency order, outermost writer first:
//
//  1. quiesce the terminal engine: kill every live PTY and JOIN the exit callbacks
//     those deaths fire. This is FIRST, and it is not a resource release (Close
//     does that) — it is the outermost writer. A dying vendor CLI's exit callback
//     is the ONLY thing that records its death: it Exits the runner and closes the
//     turn the CLI abandoned. It runs on the terminal engine's reap goroutine, so
//     leaving it to Close — after steps 2-3 and the adapter's DB close — is a
//     RACE, and one that was lost in production ("close abandoned turn: get chat …
//     sql: database is closed"). Losing it is worse than a lost warning: the two
//     writes fall on opposite sides of the teardown, so the runner's Exit commits
//     while its turn is NOT closed — and with no live runner row left, the next
//     boot's ReconcileRunnersOnBoot has nothing to reconcile, so NOTHING ever
//     closes that turn. The chat spins forever, across every restart, and takes
//     the workspace's working overlay with it;
//  2. close the shared reactor drain gate (Cancel) so no post-commit reactor
//     wireCallbacks registered starts new work;
//  3. wait out the in-flight reactors, BOUNDED by ctx — a hung reactor cannot
//     wedge shutdown past the deadline (quiver's drainWg.Wait() is unbounded; we
//     bound it, decision 11);
//  4. Shutdown each per-type asynx singleton, draining its command/projection
//     pool (itself ctx-bounded) so no event is half-processed when the adapter
//     WAL-checkpoints and closes the event/read/view DBs. Step 1's writes are
//     folded into the read models here, which is the other half of why it must
//     precede this: an Exit sent AFTER an asynx Shutdown is simply rejected
//     ("asynx: shutting down") and the death goes unrecorded.
//
// Note what step 1 is NOT: it is not a second opinion about liveness. The PTY
// remains the sole authority — we kill the process and let its death carry the
// runner away, exactly as every other teardown path does. All we have changed is
// that the daemon now WAITS to hear the news before it dismantles the ears.
//
// Realtime resources (file watchers, LSP hosts) are released separately by Close.
// Every wait honors ctx, so the whole drain is bounded by the caller's deadline.
func (c *Container) Shutdown(
	ctx context.Context,
) error {
	// FIRST, before anything below can be cut short by ctx: this is the only
	// moment the agent tool counters are ever read.
	c.logAgentToolUsage()

	c.quiesceTerminal(ctx)

	drain := c.Repositories.Drain()
	drain.Cancel() // close the gate: reactors observing drainCtx stop starting new work.

	// Bounded by ctx: a stuck reactor delays shutdown by the deadline and no longer.
	//
	// The gate — not a bare WaitGroup — is what makes this safe. Step 1 above kills the
	// PTYs, and those deaths COMMIT EVENTS, which wake reactors: the daemon is at its most
	// eventful in the very instant it is trying to go quiet. A reactor Adding on asynx's
	// bus goroutine while this line Waits is textbook WaitGroup misuse, and `-race` caught
	// it. drain.Gate makes admitting work and beginning to drain one critical section.
	drain.Gate.Wait(ctx)

	// Order matters on the ABANDON path (the clean path already drained every reaper in
	// quiesceTerminal above, so all its writes have landed). If a reaper is still in flight
	// when we get here — quiesceTerminal was cut short by ctx — it writes runners.Exit
	// FIRST and chats.StopTurn SECOND (reconcileRunnerExit), and it RETURNS EARLY if the
	// first write is rejected. So shut the RUNNER store down before the CHAT store: a
	// reaper that loses the race then has its Exit rejected and bails before StopTurn,
	// leaving the runner row LIVE for the next boot's reconcile to retry both writes. The
	// reverse order lets Exit commit while StopTurn is rejected — runner gone, turn
	// stranded, and boot reconcile has no live row left to find. Recoverable vs permanent.
	return errors.Join(
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentRunner.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
		c.axAgentActivity.Shutdown(ctx),
		c.axNode.Shutdown(ctx),
	)
}

// logAgentToolUsage emits the one and only read of the agent capability
// surface's call counters, as a single line at shutdown:
//
//	agent tool usage this boot  tools="post_review_comment=7/1 set_chat_title=3/0"
//
// (tool=calls/failures, sorted, so consecutive boots diff cleanly.)
//
// The counters exist to settle whether agents actually USE these tools — the
// shell command this surface replaces is known to be ignored by real models —
// and a counter nothing reads settles nothing. Shutdown is the right and only
// place: the numbers are cumulative over a daemon's lifetime, so this is the
// moment they are complete. A boot that saw no tool call logs nothing rather
// than an empty line.
//
// Deliberately not an HTTP route. This is a diagnostic about the daemon, not a
// resource of the product, and nothing in the UI consumes it.
func (c *Container) logAgentToolUsage() {
	if c.Usecases == nil {
		return
	}
	stats := c.Usecases.AgentToolMetrics()
	if len(stats) == 0 {
		return
	}
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d/%d", name, stats[name].Calls, stats[name].Failures))
	}
	slog.Info("agent tool usage this boot (tool=calls/failures)", "tools", strings.Join(parts, " "))
}

// quiesceTerminal runs Shutdown's step 1 — kill the PTYs, join their exit callbacks
// — BOUNDED by ctx, mirroring how the reactor drain below it is bounded (decision
// 11): a reaper wedged on something we do not control must not hold the daemon past
// its deadline. The quiesce is latched in the engine container, so the later
// engines.Close() never re-enters (and so never re-wedges) whatever we abandon here.
func (c *Container) quiesceTerminal(
	ctx context.Context,
) {
	if c.engines == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.engines.QuiesceTerminal()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Close tears down the application layer's live realtime resources: it stops
// every file watcher and LSP host the service still holds, and — on whichever
// path reaches Close WITHOUT a preceding Shutdown (harness.crash's simulated
// death; a production Serve failure that returns before Run's ctx.Done branch
// ever runs) — stops the six per-type asynx singletons' own background worker
// pools too, so neither path leaks them into whatever the process does next.
// It is idempotent and runs on graceful shutdown so fsnotify file descriptors
// and LSP subprocesses are released promptly.
func (c *Container) Close() {
	shutdownAgentRunners(c.Usecases)
	c.Realtime.Close()
	c.stopBackgroundWorkers(context.Background())
}

// stopBackgroundWorkers stops each per-type asynx singleton's own worker/
// dispatcher goroutines (8 shards x 8 workers plus dispatchers apiece, spun up
// by asynx.Builder at construction — see newAsynx) WITHOUT running Shutdown's
// write-path steps (terminal quiesce, reactor drain): those exist to let a
// GRACEFUL stop record every in-flight death before the stores close, and
// Close running them again here would be meaningless at best (nothing is
// listening any more, the graceful path already ran them) and unsafe at worst
// on the paths that reach Close first (a stray write racing an adapter that
// may already be closed).
//
// A Shutdown that already ran for this Container makes every call here an
// immediate, harmless no-op (asynx.ErrAlreadyShuttingDown, swallowed) — each
// per-type Shutdown latches via its own CompareAndSwap, so Close can call this
// unconditionally instead of tracking whether Shutdown ran. On the path that
// DIDN'T run one (a crash, a Serve failure), this is what stops the shard
// pools rather than leaving them idling on an empty queue forever: they were
// the one background resource Close used to leave for the caller to leak.
func (c *Container) stopBackgroundWorkers(
	ctx context.Context,
) {
	_ = c.axWorkspace.Shutdown(ctx)
	_ = c.axReviewThread.Shutdown(ctx)
	_ = c.axAgentRunner.Shutdown(ctx)
	_ = c.axAgentChat.Shutdown(ctx)
	_ = c.axAgentActivity.Shutdown(ctx)
	_ = c.axNode.Shutdown(ctx)
}

// shutdownAgentRunners kills every live api-transport connection before the
// daemon exits. It is the shutdown-time mirror of reconcileAgentRunners:
// nothing else in Close's chain (engine.Container.Close, Realtime.Close)
// reaches this registry, so without this call every codex-style serve
// process outlives the daemon that spawned it.
func shutdownAgentRunners(ucs *usecases.Container) {
	if ucs.AgentRunner == nil {
		return
	}
	ucs.AgentRunner.ShutdownAPIConnections()
}

// abandon stops what a New that failed with cause had started: the reactors,
// then the aggregates' pools. No writer outlives it into the stores the caller
// closes next, even when cause is ctx giving up: the teardown ignores ctx.
func (c *Container) abandon(
	ctx context.Context,
	cause error,
) error {
	ctx = context.WithoutCancel(ctx)
	err := c.Shutdown(ctx)
	shutdownAgentRunners(c.Usecases)
	c.stopBackgroundWorkers(ctx)
	return errors.Join(cause, err)
}
