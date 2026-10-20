package turn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (t *Turns) recordMessageDelta(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	if ev.Delta == nil {
		return nil
	}
	message, ok := t.messages.Observe(
		chat.ID, runner.ID, ev.Delta.TurnID, ev.Delta.MessageID,
		ev.Delta.Index, ev.Delta.Sequenced, ev.Delta.Final, ev.Delta.Text, time.Now(),
	)
	if !ok {
		return nil
	}

	if t.feed.MessageDelta != nil {
		// The empty kind is the ANSWER — the stream that existed before there was
		// more than one, and the only one that is ever recorded.
		t.feed.MessageDelta(chat.ID, chat.WorkspaceID, message.ID, message.Text, "", message.FirstAt)
	}
	// Hook processes race, so the closing increment can land before an earlier
	// one: record once whole. The turn close records whatever is still partial.
	if !message.Final || !message.Complete || message.Text == message.RecordedText {
		return nil
	}
	answer, joins, err := t.answeredTurn(ctx, chat.ID, runner.ID, ev.Delta.TurnID)
	if err != nil {
		return err
	}
	if joins {
		return t.joinAnswer(ctx, chat, runner, answer, message)
	}
	err = t.recordAssistantMessage(ctx, chat, runner, message.ID, message.Text, "", true, "completed", "", message.FirstAt)
	if err != nil {
		return err
	}
	t.messages.MarkRecorded(chat.ID, message.ID, message.Text)
	return nil
}

// answeredTurn is the reply runnerID's turn closed on, when a final message of
// that same turn lands after it: the chat has started no turn since, and the
// message names no other.
func (t *Turns) answeredTurn(ctx context.Context, chatID, runnerID, providerTurnID string) (stream.Answer, bool, error) {
	answer, ok := t.messages.Answer(chatID, runnerID)
	if !ok {
		return stream.Answer{}, false, nil
	}
	st, err := t.ledger.State(ctx, chatID)
	if err != nil {
		return stream.Answer{}, false, fmt.Errorf("agent: record streamed message: read ledger: %w", err)
	}
	last, has := st.LastTurn()
	return answer, has == answer.HasTurn && last.ID == answer.TurnID && last.Names(providerTurnID), nil
}

// joinAnswer records message as the reply its turn closed on, under that reply's
// id, keeping the fuller text of the two.
func (t *Turns) joinAnswer(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, answer stream.Answer, message stream.Message,
) error {
	text := message.Text
	if len(answer.Text) > len(text) {
		text = answer.Text
	}
	err := t.recordAssistantMessage(
		ctx, chat, runner, answer.MessageID, text, answer.Effort, false, answer.Status, answer.Diff, message.FirstAt,
	)
	if err != nil {
		return err
	}
	t.messages.Joined(chat.ID, message.ID)
	return nil
}

func (t *Turns) recordAssistantMessage(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	messageID, text, effort string,
	reopen bool,
	status string,
	diff string,
	messageAt time.Time,
) error {
	if text == "" {
		return nil
	}
	if err := t.activity.CloseTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     assistantTurnID(messageID),
		ProviderID: runner.ProviderID,
		RunnerID:   runner.ID,
		SessionID:  t.boundSession(ctx, chat.ID, runner.ProviderID),
		Text:       text,
		Effort:     effort,
		Status:     status,
		Diff:       diff,
		ItemIndex:  t.messages.IndexOf(chat.ID, runner.ID, messageID),
		OmitPlan:   reopen,
		MessageAt:  messageAt,
		Now:        time.Now(),
	}); err != nil {
		return fmt.Errorf("agent: record assistant message: %w", err)
	}
	if reopen {
		t.openAssistantTurnWithPlan(ctx, chat, runner, true)
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
	// Runner-scoped: a DIFFERENT runner's still-open message (an interrupted
	// turn, still gracefully finishing after a provider switch) must never
	// be swept up and recorded under THIS runner's provider.
	streamed := t.messages.Open(chat.ID, runner.ID)
	defer t.messages.Forget(chat.ID, runner.ID)
	// The thinking and the tool output belonged to the turn that is now ending,
	// and the answer has superseded them. Nothing durable is dropped here — a live
	// stream is never recorded (see livetext.go).
	defer t.live.forget(chat.ID)
	diff := t.live.snapshot(chat.ID, DeltaKindDiff, ev.TurnID)

	var lastRecorded string
	status := "completed"
	if ev.Kind == engineagents.HookTurnFailed {
		status = "failed"
	} else if ev.TurnStatus != "" {
		status = ev.TurnStatus
	}
	switch status {
	case "completed", "failed", "interrupted", "abandoned":
	default:
		// The provider signalled an end but its new status is not one the
		// descriptor mapped. Preserve the non-successful uncertainty.
		status = "interrupted"
	}
	for i, message := range streamed {
		text := message.Text
		last := i == len(streamed)-1
		if last && ev.Message != "" && text != ev.Message {
			if text != "" {
				slog.WarnContext(ctx, "agent: streamed message differs from the terminating hook",
					"chat_id", chat.ID, "message_id", message.ID,
					"streamed_bytes", len(text), "hook_bytes", len(ev.Message))
			}
			// The hook's report can be the fuller answer (a dropped increment)
			// or a truncated subset of what was already shown: never let a
			// shorter report overwrite text the user saw.
			if len(ev.Message) > len(text) {
				text = ev.Message
			}
		}
		if text == "" || text == message.RecordedText && !last {
			continue
		}
		effort := ""
		if last {
			effort = ev.Effort
		}
		messageDiff := ""
		if last {
			messageDiff = diff
		}
		messageStatus := "completed"
		if last {
			messageStatus = status
		}
		if err := t.recordAssistantMessage(ctx, chat, runner, message.ID, text, effort, !last, messageStatus, messageDiff, message.FirstAt); err != nil {
			return fmt.Errorf("agent: ingest hook: close turn: %w", err)
		}
		lastRecorded = text
	}

	// Only when nothing streamed at all is the hook's own text the answer.
	if ev.Message != "" && lastRecorded == "" {
		return t.answerFromHook(ctx, chat, runner, stream.Answer{
			MessageID: "hook-" + uuid.NewString(), Text: ev.Message, Effort: ev.Effort, Status: status, Diff: diff,
		})
	}
	if lastRecorded == "" {
		if err := t.activity.Abandon(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: close empty turn: %w", err)
		}
	}
	return nil
}

// answerFromHook records the closing hook's own text as its turn's reply. The
// turn's final message can land after its close: it joins this reply then.
func (t *Turns) answerFromHook(ctx context.Context, chat domain.Chat, runner engineagents.Runner, answer stream.Answer) error {
	st, err := t.ledger.State(ctx, chat.ID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: close turn: read ledger: %w", err)
	}
	last, has := st.LastTurn()
	answer.TurnID, answer.HasTurn = last.ID, has
	err = t.recordAssistantMessage(
		ctx, chat, runner, answer.MessageID, answer.Text, answer.Effort, false, answer.Status, answer.Diff, time.Time{},
	)
	if err != nil {
		return err
	}
	t.messages.Answered(chat.ID, runner.ID, answer)
	return nil
}

// AbandonMessageForRunner salvages runner's own streamed but unfinished
// message before its turn is torn down. The runner is named, not resolved:
// by then the chat may have none, or another. Unfinished is checked before
// the chat is read, so the ordinary exit that streamed nothing costs nothing.
func (t *Turns) AbandonMessageForRunner(
	ctx context.Context,
	chatID string,
	runner engineagents.Runner,
) (bool, error) {
	if len(t.messages.Unfinished(chatID, runner.ID)) == 0 {
		return false, nil
	}
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: abandon message for runner: chat: %w", err)
	}
	return t.salvageUnfinished(ctx, chat, runner)
}

// salvageUnfinished records runner's own still-open streamed message, if it
// has grown past what was already durable, so a turn torn down mid-stream
// does not lose text Crowbar already received and broadcast. Scoped to THIS
// runner — never sweeps up a different runner's still-open message, the same
// cross-attribution bug closeAssistantTurn guards against.
func (t *Turns) salvageUnfinished(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
) (bool, error) {
	recorded := false
	for _, message := range t.messages.Unfinished(chat.ID, runner.ID) {
		text := message.Text
		if text == "" || text == message.RecordedText {
			continue
		}
		if err := t.recordAssistantMessage(ctx, chat, runner, message.ID, text, "", false, "interrupted", "", message.FirstAt); err != nil {
			return recorded, err
		}
		t.messages.MarkRecorded(chat.ID, message.ID, text)
		recorded = true
	}
	t.messages.Forget(chat.ID, runner.ID)
	return recorded, nil
}
