package repositories

import (
	"context"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the four aggregate repositories.
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	AgentRun     agentrun.AgentRun
	ReviewThread reviewthread.ReviewThread
}

// New builds all aggregate repositories from their asynx instances.
func New(
	axWorkspace asynx.Asynx[domain.Workspace],
	axChat asynx.Asynx[domain.Chat],
	axAgentRun asynx.Asynx[domain.AgentRun],
	axReviewThread asynx.Asynx[domain.ReviewThread],
) *Container {
	return &Container{
		Workspace:    workspace.New(axWorkspace),
		Chat:         chat.New(axChat),
		AgentRun:     agentrun.New(axAgentRun),
		ReviewThread: reviewthread.New(axReviewThread),
	}
}

// RegisterHubProjections is the Wave-0 stub. Asynx subscriptions → hub.BroadcastX
// are wired fully in Wave 3 (03 §7).
func (c *Container) RegisterHubProjections(
	_ hub.WebSocketHub,
) error {
	return nil
}

// RecoverOrphans runs AgentRun crash recovery (running→error) and then the
// idempotent chat reconcile. Wave 0 has no read model to enumerate candidates, so
// the ID lists are empty; the enumerators are wired in a later wave (00 §6.2).
func (c *Container) RecoverOrphans(
	ctx context.Context,
) {
	RecoverAgentRuns(ctx, nil, c.AgentRun)
	ReconcileChats(ctx, nil, func(string) bool { return false }, c.Chat)
}
