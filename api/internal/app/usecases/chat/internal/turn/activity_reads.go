package turn

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// RecordTelemetry records a usage report runner sent over its hook channel. It
// is no conversation event, so it is never run through the ownership guard.
func (t *Turns) RecordTelemetry(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	raw []byte,
) error {
	chat, err := t.chatForRunner(ctx, runner)
	if err != nil {
		return err
	}
	report, err := agent.ParseTelemetry(raw, time.Now())
	if err != nil {
		return fmt.Errorf("agent: telemetry: %w: %w", apperr.ErrUnprocessable, err)
	}
	if report.Empty() {
		return nil
	}
	t.telemetry.Set(ctx, chat.ID, report)
	// Pushed, not polled: the gauge moves when the provider reports, which is
	// about once a turn. Held to the same surface gate the GET applies — a chat
	// on a surface whose channel carries no report shows no gauge at all.
	if t.feed.Telemetry != nil && t.runners.TelemetryOnChatSurface(ctx, chat.ID) {
		t.feed.Telemetry(chat.ID, chat.WorkspaceID, report)
	}
	return nil
}

func (t *Turns) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return t.telemetry.Get(chatID)
}

type ChatActivity struct {
	Turns         []domain.ActivityTurn
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
	var turns []domain.ActivityTurn
	var err error
	if after > 0 {
		turns, err = t.activity.Turns(ctx, chatID, after, 0, limit)
		if err != nil {
			return ChatActivity{}, fmt.Errorf("agent: read activity: turns: %w", err)
		}
		calls, err = t.activity.ToolCalls(ctx, chatID, after, limit)
	} else {
		turns, err = t.activity.TurnsBefore(ctx, chatID, 0, limit)
		if err != nil {
			return ChatActivity{}, fmt.Errorf("agent: read activity: turns: %w", err)
		}
		calls, err = t.activity.ToolCallsBefore(ctx, chatID, 0, limit)
	}
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: tool calls: %w", err)
	}
	subagents, err := t.activity.Subagents(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: subagents: %w", err)
	}
	interruptions, err := t.activity.Interruptions(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: interruptions: %w", err)
	}
	choices, err := t.activity.Choices(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: choices: %w", err)
	}
	return ChatActivity{
		Turns:         turns,
		ToolCalls:     calls,
		Subagents:     subagents,
		Interruptions: interruptions,
		Choices:       choices,
	}, nil
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
