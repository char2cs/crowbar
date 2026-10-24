// Package main is a dev-only fixture seeder: it writes a synthetic agent chat
// (alternating user/assistant turns, each assistant turn closing with N
// finished tool calls) directly into the same stores the real daemon reads,
// bypassing the agent runner entirely. Mirrors the throwaway, must-not-ship
// convention of cmd/crowbar-seed, for a different domain (agent chat turns and
// tool calls, not git/review fixtures) — see that command's own header comment
// for the shared convention. Built with `go run -tags noEmbed`; never linked
// into the shipped binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errNoProvider is a home with no registered descriptor to seed a chat on.
var errNoProvider = errors.New("crowbar-seed-chat: no provider is registered")

// seedOptions describes one synthetic chat. An empty ProviderID is the first
// registered descriptor's.
type seedOptions struct {
	WorkspaceID      string
	ChatID           string
	ProviderID       string
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

// firstProvider is the id of the first descriptor registered under homeDir.
func firstProvider(ctx context.Context, homeDir string) (string, error) {
	engine := engineagents.New()
	defer engine.Close()
	agents, err := engine.List(ctx, homeDir)
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: list providers: %w", err)
	}
	if len(agents) == 0 {
		return "", errNoProvider
	}
	return agents[0].ID(), nil
}

// seedChat writes opts.Turns alternating user/assistant turns — each
// assistant turn closing with opts.ToolCallsPerTurn finished tool calls —
// into the chat store and the chat's ledger. Returns the seeded chat's ID
// (opts.ChatID if set, else a generated one).
func seedChat(ctx context.Context, adapters *adapter.Container, opts seedOptions) (string, error) {
	chatID := opts.ChatID
	if chatID == "" {
		chatID = "seed-" + uuid.NewString()
	}
	provider := opts.ProviderID
	if provider == "" {
		var err error
		if provider, err = firstProvider(ctx, adapters.Home().Root()); err != nil {
			return "", err
		}
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
	now := time.Now()
	if _, err := chatStore.Create(ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: opts.WorkspaceID, Type: domain.ChatTypeChat,
		ProviderID: provider, Now: now,
	}); err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: create chat: %w", err)
	}
	title := fmt.Sprintf("Perf fixture: %d turns / %d tool calls", opts.Turns, opts.ToolCallsPerTurn)
	if _, err := chatStore.SetTitle(ctx, chatID, title, "user"); err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: set title: %w", err)
	}

	clock := func() time.Time {
		now = now.Add(100 * time.Millisecond)
		return now
	}
	chatLedger, err := ledgerstore.Open(adapters.ChatLedgerDB(), clock)
	if err != nil {
		return "", fmt.Errorf("crowbar-seed-chat: ledger: %w", err)
	}
	for i := 0; i < opts.Turns; i++ {
		rows, err := turnRows(chatID, provider, i, opts.ToolCallsPerTurn)
		if err != nil {
			return "", err
		}
		if _, _, err := chatLedger.Append(ctx, chatID, rows...); err != nil {
			return "", fmt.Errorf("crowbar-seed-chat: turn %d: %w", i, err)
		}
	}
	axAgentChat.WaitPublish()
	return chatID, nil
}

// turnRows are turn i as ledger rows: the prompt that opened it, its tool
// calls, its reply and its end.
func turnRows(chatID, provider string, i, toolCalls int) ([]ledger.Row, error) {
	turnID := uuid.NewString()
	var rows []ledger.Row
	add := func(kind ledger.Kind, body any) error {
		row, err := ledger.NewRow(kind, turnID, "", body)
		rows = append(rows, row)
		return err
	}
	errs := []error{
		add(ledger.KindUserMessage, ledger.UserMessage{Text: fmt.Sprintf("Synthetic prompt #%d", i), Origin: ledger.OriginProvider}),
		add(ledger.KindTurnStarted, ledger.TurnStarted{Provider: provider}),
	}
	for j := 0; j < toolCalls; j++ {
		toolID := fmt.Sprintf("%s-tool-%d-%d", chatID, i, j)
		target := fmt.Sprintf("file_%d_%d.go", i, j)
		errs = append(errs,
			add(ledger.KindToolCall, ledger.ToolCall{ToolID: toolID, Phase: ledger.ToolPre, Name: "Read", Target: target}),
			add(ledger.KindToolCall, ledger.ToolCall{
				ToolID: toolID, Phase: ledger.ToolPost, Status: domain.ToolStatusOK, DurationMS: 50,
			}))
	}
	errs = append(errs,
		add(ledger.KindAssistantText, ledger.AssistantText{
			MessageID: ledger.ReplyID(turnID), Index: ledger.IndexComplete,
			Text: fmt.Sprintf("Synthetic reply #%d, with **markdown** and `code`.", i),
		}),
		add(ledger.KindTurnEnded, ledger.TurnEnded{Status: string(ledger.TurnStatusDone)}))
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: turn %d: %w", i, err)
	}
	return rows, nil
}
