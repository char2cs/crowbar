package repositories

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (c *Container) SetAgentWorking(ctx context.Context, wsID, chatID string, working bool) {
	if wsID == "" || !c.setAgentTurn(wsID, chatID, working) {
		return
	}
	c.scheduleAgentBroadcast(ctx, wsID)
}

func (c *Container) forgetAgentWorking(ctx context.Context, evt asynxModels.Event[domain.Chat]) {
	c.SetAgentWorking(ctx, evt.Aggregate.WorkspaceID, evt.AggregateID, false)
	c.ownerForgotten(ctx, evt.Aggregate)
}

func (c *Container) setAgentTurn(wsID, chatID string, working bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	before := len(c.agentWorking[wsID]) > 0
	set := c.agentWorking[wsID]
	switch {
	case working && set == nil:
		c.agentWorking[wsID] = map[string]struct{}{chatID: {}}
	case working:
		set[chatID] = struct{}{}
	default:
		delete(set, chatID)
		if len(set) == 0 {
			delete(c.agentWorking, wsID)
		}
	}
	return before != (len(c.agentWorking[wsID]) > 0)
}

func (c *Container) agentWorkingFor(wsID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.agentWorking[wsID]) > 0
}

func (c *Container) scheduleAgentBroadcast(ctx context.Context, wsID string) {
	c.mu.Lock()
	_, running := c.agentBroadcasts[wsID]
	c.agentBroadcasts[wsID] = true
	c.mu.Unlock()
	if running {
		return
	}
	if !c.drainGate.Enter() {
		c.mu.Lock()
		delete(c.agentBroadcasts, wsID)
		c.mu.Unlock()
		return
	}
	go c.runAgentBroadcasts(context.WithoutCancel(ctx), wsID)
}

func (c *Container) runAgentBroadcasts(ctx context.Context, wsID string) {
	defer c.drainGate.Leave()
	c.drainGate.Proceed(ctx)
	for c.takeAgentBroadcast(wsID) {
		c.Republish(ctx, wsID)
	}
}

func (c *Container) takeAgentBroadcast(wsID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.agentBroadcasts[wsID] {
		delete(c.agentBroadcasts, wsID)
		return false
	}
	c.agentBroadcasts[wsID] = false
	return true
}
