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

// Writing the agent's messages down as the provider streams them.

// recordMessageDelta folds one increment in, and records the message the moment
// the provider says it is complete.
//
// Recording at COMPLETION rather than at turn end is the entire point. A message
// the agent finished before reaching for a tool is finished; waiting for the turn
// to end before writing it down is what made everything before the last tool call
// invisible, and — when the turn was interrupted and never ended at all — lost.
//
// Every failure is logged and swallowed, like the rest of the observation path: a
// missing message is a gap in the record, and failing the hook would break the
// vendor CLI's turn.
func (u *Usecase) recordMessageDelta(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) {
	if ev.Delta == nil {
		return
	}
	buffer, ok := u.messages.observe(
		chat.ID, ev.Delta.TurnID, ev.Delta.MessageID,
		ev.Delta.Index, ev.Delta.Final, ev.Delta.Text, time.Now(),
	)
	if !ok {
		return
	}
	// Publish what has been said SO FAR, on every increment. This never touches
	// the ledger: a message still growing is a view, and persisting each increment
	// would be roughly 1.4 durable writes a second to store text replaced a moment
	// later. The text is cumulative rather than incremental so a client that missed
	// a frame is correct again on the next one.
	if u.messageDelta != nil {
		u.messageDelta(chat.ID, chat.WorkspaceID, buffer.ID, buffer.Text())
	}
	if !buffer.Final {
		return
	}
	if !buffer.Complete() {
		// The provider said this message is done and an increment of it never
		// arrived. It is still recorded — a partial answer beats none — but the hole
		// is real and the terminating hook's copy will supersede it.
		slog.WarnContext(ctx, "agent: assistant message is missing an increment",
			"chat_id", chat.ID, "message_id", buffer.ID)
	}
	u.note(ctx, "record streamed message",
		u.recordAssistantMessage(ctx, chat, runner, buffer.ID, buffer.Text(), "", true))
	u.messages.markRecorded(chat.ID, buffer.ID, buffer.Text())
}

// recordAssistantMessage closes one assistant message onto its own row and, when
// asked, re-opens a successor for whatever the agent does next.
//
// The row is keyed by the PROVIDER'S message id, which makes it naturally stable:
// a redelivered increment rewrites the row it already wrote instead of appending a
// second copy of the same reply, and no derivation from a hook delivery id is
// needed to get there.
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
		// A closed turn leaves nothing open, and everything the agent does next —
		// the tools of the following segment, the reply still to come — needs a turn
		// to attach to. Re-opening immediately is what keeps that activity from
		// minting an anonymous turn of its own.
		u.openAssistantTurn(ctx, chat, runner)
	}
	return nil
}

// assistantTurnID keys a reply row by the provider's own message identity.
func assistantTurnID(messageID string) string { return "msg-" + messageID }

// closeAssistantTurn completes the reply with EVERYTHING the agent said.
//
// The terminating hook carries ONE message — claude's is `last_assistant_message`,
// literally the last one — so it can never be the whole record of a turn that
// spoke more than once. What it CAN be is a free reconciliation pass over the
// message it does carry, and that is how it is used here.
//
// Three cases, in the order they are handled:
//
//  1. A provider that streams. Its completed messages are already on their own
//     rows, written the moment each finished. Anything still unfinished is flushed
//     now, and the hook's own copy of the last message supersedes what was
//     assembled from increments — measured byte-identical, so a difference means
//     increments were lost and the provider's copy is the truth.
//
//  2. A provider that does not stream. Nothing was buffered, so the hook's message
//     is recorded exactly as it always was: one message, one row. codex is this
//     provider today.
//
//  3. Nothing to record at all. The turn is abandoned so its in-flight tools stop
//     reading as running, or — when every word was already written down — closed
//     onto the row the last message landed on, so the tools that ran after the
//     agent's final word are not orphaned onto a placeholder nobody wrote under.
func (u *Usecase) closeAssistantTurn(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) error {
	streamed := u.messages.openMessages(chat.ID)
	defer u.messages.forget(chat.ID)

	var lastRecorded string
	for i, buffer := range streamed {
		text := buffer.Text()
		last := i == len(streamed)-1
		if last && ev.Message != "" && text != ev.Message {
			// The hook's copy wins. It is the provider's own report of the same
			// message and it arrives complete, so a mismatch is increments that never
			// made it — exactly the hole Crowbar cannot otherwise detect a cause for.
			if text != "" {
				slog.WarnContext(ctx, "agent: streamed message differs from the terminating hook",
					"chat_id", chat.ID, "message_id", buffer.ID,
					"streamed_bytes", len(text), "hook_bytes", len(ev.Message))
			}
			text = ev.Message
		}
		if text == "" || text == buffer.recordedText && !last {
			continue
		}
		effort := ""
		if last {
			effort = ev.Effort
		}
		if err := u.recordAssistantMessage(ctx, chat, runner, buffer.ID, text, effort, !last); err != nil {
			return fmt.Errorf("agent: ingest hook: close turn: %w", err)
		}
		lastRecorded = text
	}

	// A provider with no streaming hook, or a turn whose final message never
	// streamed at all: the hook's message is all there is, and recording it is what
	// Crowbar did before any of this existed.
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

// hookMessageID keys a message that arrived on a terminating hook rather than as
// a stream, so a redelivery of that hook rewrites one row instead of appending a
// second copy of the reply.
func hookMessageID(ctx context.Context) string { return "hook-" + turnID(ctx) }

// closeTurnFromFailure ends a turn the provider could not run, with its own
// reason.
//
// This is a DIFFERENT hook from the one that ends a healthy turn, and the two are
// mutually exclusive on the wire: a turn that fails fires this and never fires
// turn_stop. So without it a failed turn is one nothing closes — a chat that spins
// forever on an agent that stopped minutes ago, indistinguishable from one that is
// genuinely working. Measured against claude 2.1.236: pointing the CLI at an
// unreachable endpoint fires this in 1.6 seconds carrying `server_error`, and
// nothing else fires at all.
//
// The reason is recorded as a NOTICE — the provider's own words about why it
// stopped, in the same family as a usage-limit banner — so the chat says what
// happened instead of just going quiet. It is carried verbatim and never
// interpreted; Crowbar does not have opinions about what `overloaded` means.
func (u *Usecase) closeTurnFromFailure(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) error {
	// Whatever the agent managed to say before it failed is still what it said.
	appendErr := u.closeAssistantTurn(ctx, chat, runner, ev)
	defer u.turns.complete(runner.ID)

	if reason := failureNotice(ev); reason != "" {
		u.note(ctx, "record turn failure", u.recordTurn(
			ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
			domain.TurnRoleNotice, reason, "",
		))
	}
	// AsyncWork is deliberately zero rather than read from the payload. A turn that
	// failed left nothing running, and the failure hook carries no such level to
	// read anyway — so the aggregate folds Working from the turn alone, which is
	// the whole point of closing it here.
	stopped, err := u.chats.StopTurn(ctx, chat.ID, time.Now(), 0)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: stop failed turn: %w", err)
	}
	u.work.set(chat.ID, stopped.Working)
	return appendErr
}

// failureNotice renders the provider's reason for the reader.
//
// Reason is a machine token — claude sends one of eleven, `rate_limit` and
// `server_error` among them — so it is shown with the provider's longer
// explanation when there is one, and alone when there is not.
func failureNotice(ev engineagents.CanonicalEvent) string {
	if ev.Failure == nil || ev.Failure.Reason == "" {
		return ""
	}
	if ev.Failure.Detail != "" {
		return ev.Failure.Reason + ": " + ev.Failure.Detail
	}
	return ev.Failure.Reason
}

// UnfinishedSince implements termwait.Messages: when the chat's oldest unfinished
// assistant message last grew.
//
// The clock it exposes starts at that message's FIRST increment, never at the
// prompt. A turn was measured spending 72.5 seconds thinking before it emitted
// anything at all, and a detector armed on prompt submission would have called
// that healthy turn interrupted.
func (u *Usecase) UnfinishedSince(chatID string) (time.Time, bool) {
	unfinished := u.messages.unfinished(chatID)
	if len(unfinished) == 0 {
		return time.Time{}, false
	}
	oldest := unfinished[0].LastAt
	for _, buffer := range unfinished[1:] {
		if buffer.LastAt.After(oldest) {
			oldest = buffer.LastAt
		}
	}
	return oldest, true
}

// AbandonMessage implements termwait.Messages: the agent was cut off mid-sentence,
// so record what it managed to say and close the turn.
//
// Recording the PARTIAL message is the part worth having. The words were really
// said and the user really saw them scroll past; throwing them away because the
// provider never got to mark the message complete would make an interrupted turn
// look like one that produced nothing. Before this, that text was lost outright.
func (u *Usecase) AbandonMessage(ctx context.Context, chatID string) (bool, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: abandon message: chat: %w", err)
	}
	runner, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		// No live runner means the exit reconcile owns this chat's turn, not us.
		return false, nil //nolint:nilerr // absence is an answer, not a failure
	}
	recorded := false
	for _, buffer := range u.messages.unfinished(chatID) {
		text := buffer.Text()
		if text == "" || text == buffer.recordedText {
			continue
		}
		if err := u.recordAssistantMessage(ctx, chat, runner, buffer.ID, text, "", false); err != nil {
			return false, err
		}
		u.messages.markRecorded(chatID, buffer.ID, text)
		recorded = true
	}
	u.messages.forget(chatID)

	// AbandonTurn rather than StopTurn, for the reason closeAbandonedTurn gives:
	// an interrupted CLI never restates the level of async work it last reported,
	// and a plain stop would leave that number standing — Working folds from the
	// turn OR that level, so the chat would spin forever on work nothing is doing.
	abandoned, err := u.chats.AbandonTurn(ctx, chatID, time.Now())
	if err != nil {
		if errors.Is(err, asynxModels.ErrValidation) {
			// The aggregate's own fold says there is no turn open. Nothing to close,
			// and nothing went wrong.
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
