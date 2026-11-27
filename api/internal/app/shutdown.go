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

func (c *Container) Shutdown(
	ctx context.Context,
) error {
	c.logAgentToolUsage()

	opsErr := c.Operations.Shutdown(ctx)
	c.Stream.Close()

	shutdownAgentRunners(c.Usecases)
	c.quiesceTerminal(ctx)

	drain := c.Repositories.Drain()
	drain.Cancel()

	drain.Gate.Wait(ctx)

	return errors.Join(
		opsErr,
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
		c.axTree.Shutdown(ctx),
	)
}

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

func (c *Container) Close() {
	shutdownAgentRunners(c.Usecases)
	c.resources.Close()
	c.stopBackgroundWorkers(context.Background())
}

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

func shutdownAgentRunners(ucs *usecases.Container) {
	if ucs == nil || ucs.AgentRunner == nil {
		return
	}
	if ucs.AgentChatFolder != nil {
		ucs.AgentChatFolder.StopMoves()
	}
	ucs.AgentRunner.ShutdownAPIConnections()
}

const WriterDrainGrace = 4 * time.Second

func (c *Container) abandon(
	ctx context.Context,
	cause error,
) error {
	return c.abandonWithin(ctx, cause, WriterDrainGrace)
}

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
