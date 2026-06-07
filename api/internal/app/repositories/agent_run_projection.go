package repositories

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RegisterAgentRunProjection drives Chat status from AgentRun lifecycle (01 §5).
// The owning workspace's agent-running overlay is refreshed separately, from
// inside the AgentRun store projection after its read model is saved, so the
// overlay never recomputes against a stale ListRunning view.
func RegisterAgentRunProjection(
	ax asynx.Asynx[domain.AgentRun],
	chats chat.Chat,
	runs agentrun.AgentRun,
) error {
	p := &agentRunProjector{chats: chats, runs: runs}
	if _, err := ax.Subscribe(asynx.Topic("agent_run.*"), p.onEvent); err != nil {
		return fmt.Errorf("agent run projection: subscribe: %w", err)
	}
	return nil
}

type agentRunProjector struct {
	chats chat.Chat
	runs  agentrun.AgentRun
}

func (p *agentRunProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentRun],
) {
	p.applyChatStatus(ctx, evt.Aggregate)
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
	if !isTerminal(run.Status) {
		return
	}
	if p.hasOtherLiveRun(ctx, run) {
		return
	}
	if _, err := p.chats.ResetIdle(ctx, run.ChatID); err != nil {
		slog.ErrorContext(ctx, "agent run projection: reset idle", "chat", run.ChatID, "err", err)
	}
}

func (p *agentRunProjector) hasOtherLiveRun(
	ctx context.Context,
	run domain.AgentRun,
) bool {
	siblings, err := p.runs.ListByChat(ctx, run.ChatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent run projection: list by chat", "chat", run.ChatID, "err", err)
		return false
	}
	for _, o := range siblings {
		if o.ID != run.ID && o.Status == domain.AgentRunStatusRunning {
			return true
		}
	}
	return false
}

func isTerminal(
	status domain.AgentRunStatus,
) bool {
	return status == domain.AgentRunStatusDone ||
		status == domain.AgentRunStatusError ||
		status == domain.AgentRunStatusInterrupted
}
