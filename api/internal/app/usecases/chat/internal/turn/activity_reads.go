package turn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (t *Turns) handleTelemetry(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	raw []byte,
) error {
	chat, ok, err := t.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}
	report, err := agent.ParseTelemetry(raw, time.Now())
	if err != nil {
		slog.DebugContext(ctx, "agent: telemetry: parse", "err", err, "provider", runner.ProviderID)
		return nil
	}
	if report.Empty() {
		return nil
	}
	t.telemetry.Set(ctx, chat.ID, report)
	return nil
}

func (t *Turns) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return t.telemetry.Get(chatID)
}

type ChatActivity struct {
	ToolCalls     []domain.ActivityToolCall
	Subagents     []domain.ActivitySubagent
	Interruptions []domain.ActivityInterruption

	Choices []domain.ActivityChoice
}

const maxActivityPage = 500

func (t *Turns) ReadActivity(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) (ChatActivity, error) {
	if _, err := t.chats.GetChat(ctx, chatID); err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: chat: %w", err)
	}
	if limit <= 0 || limit > maxActivityPage {
		limit = maxActivityPage
	}
	var calls []domain.ActivityToolCall
	var err error
	if after > 0 {
		calls, err = t.activity.ToolCalls(ctx, chatID, after, limit)
	} else {
		calls, err = t.activity.ToolCallsBefore(ctx, chatID, 0, limit)
	}
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: tool calls: %w", err)
	}
	now := time.Now()
	// Both halves through their staleness nets, so what the shelf draws agrees
	// with what OpenWork counts — see withStaleToolCallsClosed.
	calls = withStaleToolCallsClosed(calls, now)
	subagents, err := t.activity.Subagents(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: subagents: %w", err)
	}
	subagents = withStaleSubagentsClosed(subagents, now)
	interruptions, err := t.activity.Interruptions(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: interruptions: %w", err)
	}
	choices, err := t.activity.Choices(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: choices: %w", err)
	}
	return ChatActivity{
		ToolCalls:     calls,
		Subagents:     subagents,
		Interruptions: interruptions,
		Choices:       choices,
	}, nil
}

// Interruptions returns chatID's durable interruption ledger — permission waits,
// notifications, compactions, and Crowbar's own switch/model/effort markers. It is
// the fallback source engineagents.ActiveProviderID needs once a chat's most
// recently active conversation is not enough: a provider that binds via its own
// connection identity never writes a conversation row at all.
func (t *Turns) Interruptions(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityInterruption, error) {
	return t.activity.Interruptions(ctx, chatID)
}

func (t *Turns) ReadPendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	if _, err := t.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read pending choices: chat: %w", err)
	}
	choices, err := t.activity.PendingChoices(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read pending choices: %w", err)
	}
	return choices, nil
}

func (t *Turns) ReadToolPayload(
	ctx context.Context,
	chatID, toolID, side string,
) ([]byte, error) {
	if _, err := t.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read tool payload: chat: %w", err)
	}
	calls, err := t.activity.ToolCalls(ctx, chatID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("agent: read tool payload: tool calls: %w", err)
	}
	for _, c := range calls {
		if c.ID != toolID {
			continue
		}
		ref := c.RequestRef
		if side == "result" {
			ref = c.ResultRef
		}
		if ref == "" {
			return nil, agentactivity.ErrNotFound
		}
		return t.activity.Payload(ctx, ref)
	}
	return nil, agentactivity.ErrNotFound
}
