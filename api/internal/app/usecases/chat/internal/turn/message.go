package turn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
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
		chat.ID, runner.ID, ev.Delta.TurnID, ev.Delta.MessageID,
		ev.Delta.Index, ev.Delta.Sequenced, ev.Delta.Final, ev.Delta.Text, time.Now(),
	)
	if !ok {
		return
	}

	if t.feed.MessageDelta != nil {
		// The empty kind is the ANSWER — the stream that existed before there was
		// more than one, and the only one that is ever recorded.
		t.feed.MessageDelta(chat.ID, chat.WorkspaceID, message.ID, message.Text, "", message.FirstAt)
	}
	// Hook processes race, so the closing increment can land before an earlier
	// one: record once whole. The turn close records whatever is still partial.
	if !message.Final || !message.Complete || message.Text == message.RecordedText {
		return
	}
	note(ctx, "record streamed message",
		t.recordAssistantMessage(ctx, chat, runner, message.ID, message.Text, "", true, "completed", "", message.FirstAt))
	t.messages.MarkRecorded(chat.ID, message.ID, message.Text)
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

// awaitStreamed is Open, except that "nothing open YET" is not the same fact
// as "nothing ever streamed" when the closing hook actually carries text:
// the increments that assemble this exact message are their own,
// independently delivered hook (message_delta), with no ordering guarantee
// against the turn-closing hook that just arrived — a race confirmed live
// 2026-08-29 (a fresh Claude reply, persisted twice: once under
// closeAssistantTurn's fallback synthesized id, once under the streamed
// message's own id that landed moments later). Waiting here — briefly,
// bounded, on a channel nothing else holds — is what tells "the increment is
// still in flight" apart from "there truly was none." A no-op, with no wait,
// whenever something is already open or the hook reports no text at all.
func (t *Turns) awaitStreamed(ctx context.Context, chatID, runnerID, hookText string) []stream.Message {
	streamed := t.messages.Open(chatID, runnerID)
	if len(streamed) > 0 || hookText == "" {
		return streamed
	}
	stepOutOfHookGate(ctx, func() {
		streamed = t.messages.AwaitOpen(chatID, runnerID, t.messageAwaitTimeout)
	})
	return streamed
}

func (t *Turns) closeAssistantTurn(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	// Runner-scoped: a DIFFERENT runner's still-open message (an interrupted
	// turn, still gracefully finishing after a provider switch) must never
	// be swept up and recorded under THIS runner's provider.
	streamed := t.awaitStreamed(ctx, chat.ID, runner.ID, ev.Message)
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
			// The hook's own report can legitimately be the fuller answer — a
			// delta race can drop increments (see awaitStreamed's own doc
			// comment) — but it can also be a truncated SUBSET of what was
			// actually streamed and already shown to the user: confirmed live
			// 2026-09-01, a full multi-paragraph reply closed under a Stop
			// hook that reported only its last paragraph, silently deleting
			// the rest from the persisted transcript. Never let a SHORTER
			// hook report overwrite text the user already saw — prefer
			// whichever is longer, the same "don't discard visible content"
			// rule AbandonMessageForRunner already applies to a torn-down
			// turn.
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

	// lastRecorded != ev.Message is NOT the right test any more: keeping the
	// longer streamed text over a shorter hook report (above) means the two
	// can legitimately differ even though the stream's own content was
	// already recorded in full — that used to be impossible, because a
	// mismatch always meant text got overwritten to ev.Message before being
	// recorded. This fallback exists for the one case that still means
	// "nothing of the hook's content made it in": nothing streamed at all.
	if ev.Message != "" && lastRecorded == "" {
		return t.recordAssistantMessage(
			ctx, chat, runner, hookMessageID(ctx), ev.Message, ev.Effort, false, status, diff, time.Time{},
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
