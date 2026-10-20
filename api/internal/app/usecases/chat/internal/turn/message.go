package turn

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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
	if err := t.continueReply(ctx, chat.ID, runner.ID, ev); err != nil {
		return err
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
		// A reply's late stream is shown under the reply's id, which its row confirms.
		shown := cmp.Or(message.Reply.MessageID, message.ID)
		t.feed.MessageDelta(chat.ID, chat.WorkspaceID, shown, message.Text, "", message.FirstAt)
	}
	// Hook processes race, so the closing increment can land before an earlier
	// one: record once whole. The turn close records whatever is still partial.
	if !message.Final || !message.Complete || message.Text == message.RecordedText {
		return nil
	}
	if sameReply(message.Reply, message.Text) {
		return t.joinAnswer(ctx, chat, runner, message.Reply, message)
	}
	err := t.recordAssistantMessage(ctx, chat, runner, message.ID, message.Text, "", true, "completed", "", message.FirstAt)
	if err != nil {
		return err
	}
	t.messages.MarkRecorded(chat.ID, message.ID, message.Text)
	return nil
}

// continueReply decides, before the message is first shown, whether ev opens the
// late stream of the reply its runner's turn closed on. Continue keeps the first
// decision: a message already open is never re-decided.
func (t *Turns) continueReply(ctx context.Context, chatID, runnerID string, ev engineagents.CanonicalEvent) error {
	d := ev.Delta
	answer, ok := t.messages.Answer(chatID, runnerID)
	if !ok || d.MessageID == "" || ev.Subagent != nil || !sameReply(answer, d.Text) {
		return nil
	}
	last, still, err := t.answersLastTurn(ctx, chatID, answer)
	if err == nil && still && last.Names(d.TurnID) {
		t.messages.Continue(chatID, runnerID, d.TurnID, d.MessageID, answer)
	}
	return err
}

// answersLastTurn is chatID's last ledger turn, and whether it is still the turn
// answer closed.
func (t *Turns) answersLastTurn(ctx context.Context, chatID string, answer stream.Answer) (ledger.Turn, bool, error) {
	st, err := t.ledger.State(ctx, chatID)
	if err != nil {
		return ledger.Turn{}, false, fmt.Errorf("agent: record streamed message: read ledger: %w", err)
	}
	last, has := st.LastTurn()
	return last, has == answer.HasTurn && last.ID == answer.TurnID, nil
}

// sameReply is whether text, as far as it has come, reads as reply: one is a
// prefix of the other. A late stream whose words turn out another's is its own.
func sameReply(reply stream.Answer, text string) bool {
	if reply.MessageID == "" {
		return false
	}
	return strings.HasPrefix(reply.Text, text) || strings.HasPrefix(text, reply.Text)
}

// recordUnfinished records a message its own final delta did not finish, as
// that delta would have: a reply's late stream joins the reply.
func (t *Turns) recordUnfinished(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, message stream.Message, status string,
) error {
	if sameReply(message.Reply, message.Text) {
		return t.joinAnswer(ctx, chat, runner, message.Reply, message)
	}
	return t.recordAssistantMessage(ctx, chat, runner, message.ID, message.Text, "", false, status, "", message.FirstAt)
}

// joinAnswer joins message to the reply its turn closed on: a message that adds
// no text to the reply writes nothing.
func (t *Turns) joinAnswer(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, answer stream.Answer, message stream.Message,
) error {
	if len(message.Text) > len(answer.Text) {
		if err := t.writeJoin(ctx, chat, runner, answer, message); err != nil {
			return err
		}
	}
	t.messages.Joined(chat.ID, message.ID)
	return nil
}

// writeJoin writes message's fuller text onto its reply's row, only while that
// reply's turn is the chat's last: a close inside the next turn would hand the
// old row that turn's place, plan and tools. The recorded answer stands then.
func (t *Turns) writeJoin(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, answer stream.Answer, message stream.Message,
) error {
	_, still, err := t.answersLastTurn(ctx, chat.ID, answer)
	if err != nil || !still {
		return err
	}
	return t.recordAssistantMessage(
		ctx, chat, runner, answer.MessageID, message.Text, answer.Effort, false, answer.Status, answer.Diff, message.FirstAt,
	)
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
	open := t.messages.Open(chat.ID, runner.ID)
	defer t.messages.Forget(chat.ID, runner.ID)
	// The thinking and the tool output belonged to the turn that is now ending,
	// and the answer has superseded them. Nothing durable is dropped here — a live
	// stream is never recorded (see livetext.go).
	defer t.live.forget(chat.ID)
	diff := t.live.snapshot(chat.ID, DeltaKindDiff, ev.TurnID)
	streamed, err := t.closeLateReplies(ctx, chat, runner, open)
	if err != nil {
		return err
	}

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

// closeLateReplies joins the late streams of replies earlier closes recorded:
// they belong to those turns, not this one. It returns the turn's own messages,
// among them a late stream whose words turned out another message's.
func (t *Turns) closeLateReplies(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, open []stream.Message,
) ([]stream.Message, error) {
	own := open[:0]
	for _, message := range open {
		if !sameReply(message.Reply, message.Text) {
			own = append(own, message)
			continue
		}
		if err := t.joinAnswer(ctx, chat, runner, message.Reply, message); err != nil {
			return nil, fmt.Errorf("agent: ingest hook: close turn: %w", err)
		}
	}
	return own, nil
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
	// The runner is ending, or leaving its session: what it streamed or answered
	// there is never this chat's to join again.
	defer t.messages.Retire(chatID, runner.ID)
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
		if err := t.recordUnfinished(ctx, chat, runner, message, "interrupted"); err != nil {
			return recorded, err
		}
		t.messages.MarkRecorded(chat.ID, message.ID, text)
		recorded = true
	}
	t.messages.Forget(chat.ID, runner.ID)
	return recorded, nil
}
