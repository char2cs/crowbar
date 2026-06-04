package repositories

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RecoverAgentRuns drives any candidate AgentRun still in running to error. Uses
// SendWait so all recovery events drain before the caller proceeds (00 §6.2).
func RecoverAgentRuns(
	ctx context.Context,
	candidateIDs []string,
	runs agentrun.AgentRun,
) {
	for _, id := range candidateIDs {
		recoverOneRun(ctx, id, runs)
	}
}

func recoverOneRun(
	ctx context.Context,
	id string,
	runs agentrun.AgentRun,
) {
	run, err := runs.Get(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: get agent run", "id", id, "err", err)
		return
	}
	if run.Status != domain.AgentRunStatusRunning {
		return
	}
	if _, failErr := runs.Fail(ctx, id); failErr != nil {
		slog.WarnContext(ctx, "crash recovery: fail agent run", "id", id, "err", failErr)
	}
}

// ReconcileChats resets any candidate Chat stuck in agent-running with no live run
// back to idle. ResetIdle is idempotent, so this is safe to run after AgentRun
// recovery regardless of which pass clears a given chat first (00 §6.2).
func ReconcileChats(
	ctx context.Context,
	candidateIDs []string,
	hasLiveRun func(chatID string) bool,
	chats chat.Chat,
) {
	for _, id := range candidateIDs {
		reconcileOneChat(ctx, id, hasLiveRun, chats)
	}
}

func reconcileOneChat(
	ctx context.Context,
	id string,
	hasLiveRun func(chatID string) bool,
	chats chat.Chat,
) {
	c, err := chats.Get(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: get chat", "id", id, "err", err)
		return
	}
	if c.Status != domain.ChatStatusAgentRunning || hasLiveRun(id) {
		return
	}
	if _, resetErr := chats.ResetIdle(ctx, id); resetErr != nil {
		slog.WarnContext(ctx, "crash recovery: reset chat", "id", id, "err", resetErr)
	}
}
