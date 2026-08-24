package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
)

func (t *Turns) recordMessageDelta(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) {
	if ev.Delta == nil {
		return
	}
	message, ok := t.messages.Observe(
		chat.ID, ev.Delta.TurnID, ev.Delta.MessageID,
		ev.Delta.Index, ev.Delta.Final, ev.Delta.Text, time.Now(),
	)
	if !ok {
		return
	}

	if t.messageDelta != nil {
		t.messageDelta(chat.ID, chat.WorkspaceID, message.ID, message.Text)
	}
	if !message.Final {
		return
	}
	if !message.Complete {
		slog.WarnContext(ctx, "agent: assistant message is missing an increment",
			"chat_id", chat.ID, "message_id", message.ID)
	}
	note(ctx, "record streamed message",
		t.recordAssistantMessage(ctx, chat, runner, message.ID, message.Text, "", true))
	t.messages.MarkRecorded(chat.ID, message.ID, message.Text)
}

func (t *Turns) recordAssistantMessage(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	messageID, text, effort string,
	reopen bool,
) error {
	if text == "" {
		return nil
	}
	if err := t.activity.CloseTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     assistantTurnID(messageID),
		ProviderID: runner.ProviderID,
		RunnerID:   runner.ID,
		SessionID:  runner.CurrentSession,
		Text:       text,
		Effort:     effort,
		Now:        time.Now(),
	}); err != nil {
		return fmt.Errorf("agent: record assistant message: %w", err)
	}
	if reopen {
		t.openAssistantTurn(ctx, chat, runner)
	}
	return nil
}

func assistantTurnID(messageID string) string { return "msg-" + messageID }

func (t *Turns) closeAssistantTurn(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	streamed := t.messages.Open(chat.ID)
	defer t.messages.Forget(chat.ID)

	var lastRecorded string
	for i, message := range streamed {
		text := message.Text
		last := i == len(streamed)-1
		if last && ev.Message != "" && text != ev.Message {
			if text != "" {
				slog.WarnContext(ctx, "agent: streamed message differs from the terminating hook",
					"chat_id", chat.ID, "message_id", message.ID,
					"streamed_bytes", len(text), "hook_bytes", len(ev.Message))
			}
			text = ev.Message
		}
		if text == "" || text == message.RecordedText && !last {
			continue
		}
		effort := ""
		if last {
			effort = ev.Effort
		}
		if err := t.recordAssistantMessage(ctx, chat, runner, message.ID, text, effort, !last); err != nil {
			return fmt.Errorf("agent: ingest hook: close turn: %w", err)
		}
		lastRecorded = text
	}

	if ev.Message != "" && ev.Message != lastRecorded {
		return t.recordAssistantMessage(
			ctx, chat, runner, hookMessageID(ctx), ev.Message, ev.Effort, false,
		)
	}
	if lastRecorded == "" {
		if err := t.activity.Abandon(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: close empty turn: %w", err)
		}
	}
	return nil
}

func hookMessageID(ctx context.Context) string { return "hook-" + inflight.RecordID(ctx) }

func (t *Turns) closeTurnFromFailure(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	appendErr := t.closeAssistantTurn(ctx, chat, runner, ev)
	defer t.turns.Complete(runner.ID)

	if reason := failureNotice(ev); reason != "" {
		note(ctx, "record turn failure", t.conversations.RecordTurn(
			ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
			domain.TurnRoleNotice, reason, "",
		))
	}

	stopped, err := t.chats.StopTurn(ctx, chat.ID, time.Now(), 0)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: stop failed turn: %w", err)
	}
	t.work.Set(chat.ID, stopped.Working)
	return appendErr
}

func failureNotice(ev engineagents.CanonicalEvent) string {
	if ev.Failure == nil || ev.Failure.Reason == "" {
		return ""
	}
	if ev.Failure.Detail != "" {
		return ev.Failure.Reason + ": " + ev.Failure.Detail
	}
	return ev.Failure.Reason
}

// UnfinishedSince reports when the chat's assistant stream last grew, and whether any
// message is still unterminated. It answers with the NEWEST increment across every
// unfinished message, not the oldest: one message still advancing means the CLI is
// alive, so the quiet period the sweep measures must restart on any of them.
func (t *Turns) UnfinishedSince(chatID string) (time.Time, bool) {
	unfinished := t.messages.Unfinished(chatID)
	if len(unfinished) == 0 {
		return time.Time{}, false
	}
	newest := unfinished[0].LastAt
	for _, message := range unfinished[1:] {
		if message.LastAt.After(newest) {
			newest = message.LastAt
		}
	}
	return newest, true
}

func (t *Turns) AbandonMessage(ctx context.Context, chatID string) (bool, error) {
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: abandon message: chat: %w", err)
	}
	runner, err := t.runnerStore.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return false, nil //nolint:nilerr // absence is an answer, not a failure
	}
	recorded := false
	for _, message := range t.messages.Unfinished(chatID) {
		text := message.Text
		if text == "" || text == message.RecordedText {
			continue
		}
		if err := t.recordAssistantMessage(ctx, chat, runner, message.ID, text, "", false); err != nil {
			return false, err
		}
		t.messages.MarkRecorded(chatID, message.ID, text)
		recorded = true
	}
	t.messages.Forget(chatID)

	abandoned, err := t.chats.AbandonTurn(ctx, chatID, time.Now())
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			t.work.Set(chatID, false)
			return recorded, nil
		}
		return recorded, fmt.Errorf("agent: abandon message: abandon turn: %w", err)
	}
	t.work.Set(chatID, abandoned.Working)
	slog.InfoContext(ctx, "agent: closed a turn whose message was cut off",
		"chat_id", chatID, "recorded_partial", recorded)
	return true, nil
}
