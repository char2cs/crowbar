package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Shutdown quiesces every writer within ctx before the adapter closes the DBs, outermost first: kill the
// PTYs and join their exit callbacks, close the reactor drain gate, wait out in-flight reactors (bounded),
// then Shutdown each asynx singleton. Exits must fold before asynx stops, or the next boot closes them.
func (c *Container) Shutdown(
	ctx context.Context,
) error {
	// FIRST, before anything below can be cut short by ctx: this is the only
	// moment the agent tool counters are ever read.
	c.logAgentToolUsage()

	// Operation work commits events and runs git, so it ends while the reactors and aggregates still run.
	opsErr := c.Operations.Shutdown(ctx)
	// Operations publish until they shut down; only then do the sockets close.
	c.Stream.Close()

	// Before the PTYs die: their exits record daemon_restart, not exited.
	shutdownAgentRunners(c.Usecases)
	c.quiesceTerminal(ctx)

	drain := c.Repositories.Drain()
	drain.Cancel() // close the gate: reactors observing drainCtx stop starting new work.

	// Bounded by ctx. The gate, not a bare WaitGroup: the PTY deaths above wake reactors while this waits,
	// and drain.Gate makes admitting work and starting to drain one critical section.
	drain.Gate.Wait(ctx)

	return errors.Join(
		opsErr,
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
		c.axTree.Shutdown(ctx),
	)
}

// logAgentToolUsage logs the agent tool call counters once at shutdown (tool=calls/failures, sorted),
// when they are complete; they exist to show whether agents use these tools. Logs nothing for a boot
// with no calls; deliberately not an HTTP route.
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

// quiesceTerminal runs Shutdown's step 1 bounded by ctx, so a wedged reaper cannot hold the daemon past
// its deadline; the engine latches the quiesce, so engines.Close() never re-wedges on it.
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

// Close stops every file watcher and LSP host a held scope runs and, on paths that skip Shutdown (a
// crash, a failed Serve), the asynx worker pools too. Idempotent.
func (c *Container) Close() {
	shutdownAgentRunners(c.Usecases)
	c.resources.Close()
	c.stopBackgroundWorkers(context.Background())
}

// stopBackgroundWorkers stops each asynx singleton's worker pools without Shutdown's write-path steps,
// which would be unsafe once the stores may be closed. After a Shutdown it is a harmless no-op
// (ErrAlreadyShuttingDown), so Close calls it unconditionally.
func (c *Container) stopBackgroundWorkers(
	ctx context.Context,
) {
	for _, err := range []error{
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
		c.axTree.Shutdown(ctx),
	} {
		if err != nil && !errors.Is(err, asynxModels.ErrAlreadyShuttingDown) {
			slog.WarnContext(ctx, "app: stop an aggregate's workers", "err", err)
		}
	}
}

// shutdownAgentRunners notes daemon_restart on every live runner and kills
// every api-transport connection, which nothing else in Close's chain reaches:
// without it each serve process outlives the daemon that spawned it.
func shutdownAgentRunners(ucs *usecases.Container) {
	if ucs == nil || ucs.AgentRunner == nil {
		return
	}
	if ucs.AgentChatFolder != nil {
		ucs.AgentChatFolder.StopMoves()
	}
	ucs.AgentRunner.ShutdownAPIConnections()
}

// WriterDrainGrace bounds a teardown's wait for the writers, so a wedged reaper
// cannot hang a daemon that has no supervisor to kill it.
const WriterDrainGrace = 4 * time.Second

// abandon stops what a New that failed with cause had started: the reactors,
// then the aggregates' pools. It ignores ctx, which may be what gave up, and
// waits on the writers for WriterDrainGrace, as a daemon's shutdown does.
func (c *Container) abandon(
	ctx context.Context,
	cause error,
) error {
	return c.abandonWithin(ctx, cause, WriterDrainGrace)
}

// abandonWithin is abandon with the writers' wait bounded by grace.
func (c *Container) abandonWithin(
	ctx context.Context,
	cause error,
	grace time.Duration,
) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancel()
	err := c.Shutdown(ctx)
	shutdownAgentRunners(c.Usecases)
	c.stopBackgroundWorkers(ctx)
	return errors.Join(cause, err)
}
