package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
)

func (u *Usecase) recordMessageDelta(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) {
	if ev.Delta == nil {
		return
	}
	message, ok := u.messages.observe(
		chat.ID, ev.Delta.TurnID, ev.Delta.MessageID,
		ev.Delta.Index, ev.Delta.Final, ev.Delta.Text, time.Now(),
	)
	if !ok {
		return
	}

	if u.messageDelta != nil {
		u.messageDelta(chat.ID, chat.WorkspaceID, message.ID, message.Text)
	}
	if !message.Final {
		return
	}
	if !message.Complete {
		slog.WarnContext(ctx, "agent: assistant message is missing an increment",
			"chat_id", chat.ID, "message_id", message.ID)
	}
	u.note(ctx, "record streamed message",
		u.recordAssistantMessage(ctx, chat, runner, message.ID, message.Text, "", true))
	u.messages.markRecorded(chat.ID, message.ID, message.Text)
}

func (u *Usecase) recordAssistantMessage(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	messageID, text, effort string,
	reopen bool,
) error {
	if text == "" {
		return nil
	}
	if err := u.activity.CloseTurn(ctx, agentactivity.TurnInput{
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
		u.openAssistantTurn(ctx, chat, runner)
	}
	return nil
}

func assistantTurnID(messageID string) string { return "msg-" + messageID }

func (u *Usecase) closeAssistantTurn(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) error {
	streamed := u.messages.openMessages(chat.ID)
	defer u.messages.forget(chat.ID)

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
		if err := u.recordAssistantMessage(ctx, chat, runner, message.ID, text, effort, !last); err != nil {
			return fmt.Errorf("agent: ingest hook: close turn: %w", err)
		}
		lastRecorded = text
	}

	if ev.Message != "" && ev.Message != lastRecorded {
		return u.recordAssistantMessage(
			ctx, chat, runner, hookMessageID(ctx), ev.Message, ev.Effort, false,
		)
	}
	if lastRecorded == "" {
		if err := u.activity.Abandon(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: close empty turn: %w", err)
		}
	}
	return nil
}

func hookMessageID(ctx context.Context) string { return "hook-" + turnID(ctx) }

func (u *Usecase) closeTurnFromFailure(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) error {
	appendErr := u.closeAssistantTurn(ctx, chat, runner, ev)
	defer u.turns.complete(runner.ID)

	if reason := failureNotice(ev); reason != "" {
		u.note(ctx, "record turn failure", u.recordTurn(
			ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
			domain.TurnRoleNotice, reason, "",
		))
	}

	stopped, err := u.chats.StopTurn(ctx, chat.ID, time.Now(), 0)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: stop failed turn: %w", err)
	}
	u.work.set(chat.ID, stopped.Working)
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
func (u *Usecase) UnfinishedSince(chatID string) (time.Time, bool) {
	unfinished := u.messages.unfinished(chatID)
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

func (u *Usecase) AbandonMessage(ctx context.Context, chatID string) (bool, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: abandon message: chat: %w", err)
	}
	runner, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return false, nil //nolint:nilerr // absence is an answer, not a failure
	}
	recorded := false
	for _, message := range u.messages.unfinished(chatID) {
		text := message.Text
		if text == "" || text == message.RecordedText {
			continue
		}
		if err := u.recordAssistantMessage(ctx, chat, runner, message.ID, text, "", false); err != nil {
			return false, err
		}
		u.messages.markRecorded(chatID, message.ID, text)
		recorded = true
	}
	u.messages.forget(chatID)

	abandoned, err := u.chats.AbandonTurn(ctx, chatID, time.Now())
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			u.work.set(chatID, false)
			return recorded, nil
		}
		return recorded, fmt.Errorf("agent: abandon message: abandon turn: %w", err)
	}
	u.work.set(chatID, abandoned.Working)
	slog.InfoContext(ctx, "agent: closed a turn whose message was cut off",
		"chat_id", chatID, "recorded_partial", recorded)
	return true, nil
}
