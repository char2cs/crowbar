package repositories

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RefreshWorkspaceFunc re-broadcasts a workspace row so its derived agent-running
// overlay refreshes after an AgentRun transition (00 §6.1).
type RefreshWorkspaceFunc func(
	ctx context.Context,
	wsID string,
)

// RegisterAgentRunProjection drives Chat status from AgentRun lifecycle (01 §5)
// and refreshes the owning workspace's overlay.
func RegisterAgentRunProjection(
	ax asynx.Asynx[domain.AgentRun],
	chats chat.Chat,
	refresh RefreshWorkspaceFunc,
) error {
	p := &agentRunProjector{chats: chats, refresh: refresh}
	if _, err := ax.Subscribe(asynx.Topic("agent_run.*"), p.onEvent); err != nil {
		return fmt.Errorf("agent run projection: subscribe: %w", err)
	}
	return nil
}

type agentRunProjector struct {
	chats   chat.Chat
	refresh RefreshWorkspaceFunc
}

func (p *agentRunProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentRun],
) {
	p.applyChatStatus(ctx, evt.Aggregate)
	p.refresh(ctx, evt.Aggregate.WsID)
}

func (p *agentRunProjector) applyChatStatus(
	ctx context.Context,
	run domain.AgentRun,
) {
	if run.Status == domain.AgentRunStatusRunning {
		if _, err := p.chats.SetAgentRunning(ctx, run.ChatID); err != nil {
			slog.ErrorContext(ctx, "agent run projection: set running", "chat", run.ChatID, "err", err)
		}
		return
	}
	if isTerminal(run.Status) {
		if _, err := p.chats.ResetIdle(ctx, run.ChatID); err != nil {
			slog.ErrorContext(ctx, "agent run projection: reset idle", "chat", run.ChatID, "err", err)
		}
	}
}

func isTerminal(
	status domain.AgentRunStatus,
) bool {
	return status == domain.AgentRunStatusDone ||
		status == domain.AgentRunStatusError ||
		status == domain.AgentRunStatusInterrupted
}
