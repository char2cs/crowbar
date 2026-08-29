// Package main is a dev-only fixture seeder: it writes a synthetic agent chat
// (alternating user/assistant turns, each assistant turn closing with N
// finished tool calls) directly into the same event-sourced stores the real
// daemon reads, bypassing the agent runner entirely. Mirrors the throwaway,
// must-not-ship convention of cmd/crowbar-seed, for a different domain (agent
// chat turns and tool calls, not git/review fixtures) — see that command's
// own header comment for the shared convention. Built with `go run -tags
// noEmbed`; never linked into the shipped binary.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// seedOptions describes one synthetic chat.
type seedOptions struct {
	WorkspaceID      string
	ChatID           string
	Turns            int
	ToolCallsPerTurn int
}

// buildAsynx mirrors internal/app.newAsynx, which is unexported and lives in
// a package this command cannot import — this command is its own main
// package, same as cmd/crowbar-seed.
func buildAsynx[T any](es asynxModels.Store, ss asynxModels.SnapshotStore) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		WithPanicHandler(func(ctx context.Context, evt asynxModels.Event[T], p any) {
			slog.ErrorContext(ctx, "crowbar-seed-chat: projection panic", "event", evt.EventName, "panic", p)
		}).
		WithPublishErrorHandler(func(ctx context.Context, evt asynxModels.Event[T], err error) {
			slog.ErrorContext(ctx, "crowbar-seed-chat: publish error", "event", evt.EventName, "err", err)
		}).
		Build()
}

// seedChat writes opts.Turns alternating user/assistant turns — each
// assistant turn closing with opts.ToolCallsPerTurn finished tool calls —
// into the chat and activity event stores. Returns the seeded chat's ID
// (opts.ChatID if set, else a generated one).
func seedChat(ctx context.Context, adapters *adapter.Container, opts seedOptions) (string, error) {
	chatID := opts.ChatID
	if chatID == "" {
		chatID = "seed-" + uuid.NewString()
	}

	axAgentChat, err := buildAsynx[domain.Chat](adapters.AgentChatES(), adapters.AgentChatSS())
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: asynx agent chat: %w", err)
	}
	chatStore, err := agentchat.NewEventSourced(
		axAgentChat, adapters.AgentChatES(), adapters.AgentChatReadDB(), nil,
	)
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: chat store: %w", err)
	}

	axAgentActivity, err := buildAsynx[domain.ChatActivity](adapters.AgentActivityES(), adapters.AgentActivitySS())
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: asynx agent activity: %w", err)
	}
	activityStore, err := agentactivity.NewEventSourced(
		axAgentActivity, adapters.AgentActivityES(), adapters.AgentActivityReadDB(),
		filepath.Join(adapters.CrowbarHome(), "state", "content"),
	)
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: activity store: %w", err)
	}

	now := time.Now()
	if _, err := chatStore.Create(ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: opts.WorkspaceID, Type: domain.ChatTypeChat, Now: now,
	}); err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: create chat: %w", err)
	}
	title := fmt.Sprintf("Perf fixture: %d turns / %d tool calls", opts.Turns, opts.ToolCallsPerTurn)
	if _, err := chatStore.SetTitle(ctx, chatID, title, "user"); err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: set title: %w", err)
	}

	for i := 0; i < opts.Turns; i++ {
		now = now.Add(time.Second)
		userTurnID := fmt.Sprintf("%s-user-%d", chatID, i)
		if err := activityStore.AppendTurn(ctx, agentactivity.TurnInput{
			ChatID: chatID, TurnID: userTurnID, Role: domain.TurnRoleUser,
			Text: fmt.Sprintf("Synthetic prompt #%d", i), Now: now,
		}); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: append user turn %d: %w", i, err)
		}

		now = now.Add(time.Second)
		assistantTurnID := fmt.Sprintf("%s-assistant-%d", chatID, i)
		if err := activityStore.OpenTurn(ctx, agentactivity.TurnInput{
			ChatID: chatID, TurnID: assistantTurnID, Role: domain.TurnRoleAssistant,
			ProviderID: "claude", Now: now,
		}); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: open assistant turn %d: %w", i, err)
		}

		for j := 0; j < opts.ToolCallsPerTurn; j++ {
			now = now.Add(100 * time.Millisecond)
			toolID := fmt.Sprintf("%s-tool-%d-%d", chatID, i, j)
			if err := activityStore.InvokeTool(ctx, agentactivity.ToolInput{
				ChatID: chatID, ToolID: toolID, Name: "Read",
				Target: fmt.Sprintf("file_%d_%d.go", i, j), Now: now,
			}); err != nil {
				return "", fmt.Errorf("crowbar-seed-chat: invoke tool %d/%d: %w", i, j, err)
			}
			now = now.Add(50 * time.Millisecond)
			if err := activityStore.CompleteTool(ctx, agentactivity.ToolResultInput{
				ChatID: chatID, ToolID: toolID, Name: "Read",
				Target: fmt.Sprintf("file_%d_%d.go", i, j),
				Status: domain.ToolStatusOK, DurationMS: 50, Now: now,
			}); err != nil {
				return "", fmt.Errorf("crowbar-seed-chat: complete tool %d/%d: %w", i, j, err)
			}
		}

		now = now.Add(time.Second)
		if err := activityStore.CloseTurn(ctx, agentactivity.TurnInput{
			ChatID: chatID, TurnID: assistantTurnID, ProviderID: "claude",
			Text: fmt.Sprintf("Synthetic reply #%d, with **markdown** and `code`.", i), Now: now,
		}); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: close assistant turn %d: %w", i, err)
		}
	}

	axAgentActivity.WaitPublish()
	axAgentChat.WaitPublish()

	return chatID, nil
}
