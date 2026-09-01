package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
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
		chat.ID, runner.ID, ev.Delta.TurnID, ev.Delta.MessageID,
		ev.Delta.Index, ev.Delta.Sequenced, ev.Delta.Final, ev.Delta.Text, time.Now(),
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
		ItemIndex:  t.messages.IndexOf(chat.ID, runner.ID, messageID),
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
func (t *Turns) awaitStreamed(chatID, runnerID, hookText string) []stream.Message {
	streamed := t.messages.Open(chatID, runnerID)
	if len(streamed) > 0 || hookText == "" {
		return streamed
	}
	return t.messages.AwaitOpen(chatID, runnerID, t.messageAwaitTimeout)
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
	streamed := t.awaitStreamed(chat.ID, runner.ID, ev.Message)
	defer t.messages.Forget(chat.ID, runner.ID)

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
	unfinished := t.messages.UnfinishedAcrossRunners(chatID)
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
	recorded, err := t.salvageUnfinished(ctx, chat, runner)
	if err != nil {
		return false, err
	}

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

// AbandonMessageForRunner salvages runner's own already-streamed-but-not-yet-
// final message before its turn is torn down by something other than the
// quiet-screen sweep AbandonMessage serves — a user Stop or a provider Switch
// retiring the CLI mid-answer. It exists because AbandonMessage's own
// LiveRunnerForChat lookup cannot be reused here: by the time a runner is
// being retired, displace() has already run, so that lookup answers "nobody"
// (or, worse, a DIFFERENT runner already spawned in its place) and would
// salvage nothing, or the wrong runner's buffer. The caller must therefore
// name the exact runner being retired.
//
// Checks Unfinished BEFORE fetching the chat: closeAbandonedTurn calls this on
// every ordinary runner exit, the overwhelming majority of which streamed
// nothing and have every reason to have already been purged (a deleted chat,
// a test double with no chat row at all) — that must stay a silent, free
// no-op rather than a GetChat call whose failure gets logged on every one of
// them.
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
		if err := t.recordAssistantMessage(ctx, chat, runner, message.ID, text, "", false); err != nil {
			return recorded, err
		}
		t.messages.MarkRecorded(chat.ID, message.ID, text)
		recorded = true
	}
	t.messages.Forget(chat.ID, runner.ID)
	return recorded, nil
}
