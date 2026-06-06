package repositories

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RecoverAgentRuns drives every read-model AgentRun still in running to error,
// using SendWait so recovery events drain before the caller proceeds (00 §6.2).
func RecoverAgentRuns(
	ctx context.Context,
	runs agentrun.AgentRun,
) {
	running, err := runs.ListRunning(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: list running runs", "err", err)
		return
	}
	for _, run := range running {
		recoverOneRun(ctx, run.ID, runs)
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

// ReconcileChats resets every chat stuck in agent-running with no live run back to
// idle. ResetIdle is idempotent, so it is safe after AgentRun recovery (00 §6.2).
func ReconcileChats(
	ctx context.Context,
	chats chat.Chat,
	runs agentrun.AgentRun,
) {
	stuck, err := chats.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: list chats", "err", err)
		return
	}
	live := liveChatSet(ctx, runs)
	for _, c := range stuck {
		if c.DeletedAt != nil {
			continue
		}
		reconcileOneChat(ctx, c.ID, func(id string) bool { return live[id] }, chats)
	}
}

func liveChatSet(
	ctx context.Context,
	runs agentrun.AgentRun,
) map[string]bool {
	live := map[string]bool{}
	running, err := runs.ListRunning(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: live set", "err", err)
		return live
	}
	for _, run := range running {
		live[run.ChatID] = true
	}
	return live
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
