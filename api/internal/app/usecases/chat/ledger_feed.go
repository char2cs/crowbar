package chat

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// publishLedgerRows advances the transcript by a commit and tells the chat's
// clients what it changed. It runs under the chat's ledger writer, so it reads
// the snapshot through Held: Get may read the ledger and wait on that writer.
func (u *Usecase) publishLedgerRows(chatID string, rows []ledger.Row, st ledger.State) {
	settled := u.transcript.Apply(chatID, rows)
	feed := u.feed.Load()
	if feed == nil {
		return
	}
	snap, err := u.snapshots.Held(context.Background(), chatID)
	if err != nil {
		slog.Warn("agent: publish ledger rows: chat snapshot", "chat_id", chatID, "err", err)
		return
	}
	wsID := snap.Chat.WorkspaceID
	asked := false
	for _, r := range rows {
		publishRow(feed, chatID, wsID, r)
		asked = asked || movesAsks(r.Kind)
	}
	for _, m := range settled {
		if m.Role == domain.TurnRoleAssistant && feed.MessageDelta != nil {
			feed.MessageDelta(chatID, wsID, strings.TrimPrefix(m.ID, transcript.MessageID("")), m.Text, "", m.At)
		}
	}
	if asked {
		u.transcript.choices.publish(feed, chatID, wsID, transcript.PendingChoicesOf(st), u.answers.AnswerableIDs)
	}
}

func publishRow(feed *ChatFeed, chatID, wsID string, r ledger.Row) {
	switch r.Kind {
	case ledger.KindPromptSettled:
		if body, err := ledger.DecodeBody[ledger.PromptSettled](r); err == nil && feed.PromptSettled != nil {
			feed.PromptSettled(chatID, wsID, body.RequestID, body.Consumed)
		}
	case ledger.KindMarker:
		body, err := ledger.DecodeBody[ledger.Marker](r)
		compaction := body.Kind == ledger.MarkerCompactionStarted || body.Kind == ledger.MarkerCompactionEnded
		if err == nil && compaction && feed.Compaction != nil {
			feed.Compaction(chatID, wsID, body.Kind == ledger.MarkerCompactionStarted)
		}
	case ledger.KindPlan:
		body, err := ledger.DecodeBody[ledger.Plan](r)
		if err != nil || r.Scope != "" || feed.Plan == nil {
			return
		}
		steps := make([]engineagents.PlanStep, 0, len(body.Items))
		for _, it := range body.Items {
			steps = append(steps, engineagents.PlanStep{Text: it.Text, Status: it.Status})
		}
		feed.Plan(chatID, wsID, steps)
	case ledger.KindUserMessage, ledger.KindTurnStarted, ledger.KindSessionBound, ledger.KindSessionLost,
		ledger.KindRunnerStarted, ledger.KindRunnerExited, ledger.KindTurnEnded, ledger.KindTurnFailed,
		ledger.KindAssistantText, ledger.KindReasoning, ledger.KindToolCall, ledger.KindAsk,
		ledger.KindAskResolved, ledger.KindSubagent:
		// published through the transcript or the snapshot, not on its own
	}
}

// movesAsks reports whether a row of kind can open or close an ask.
func movesAsks(kind ledger.Kind) bool {
	return slices.Contains([]ledger.Kind{
		ledger.KindAsk, ledger.KindAskResolved, ledger.KindTurnEnded, ledger.KindTurnFailed, ledger.KindRunnerExited,
	}, kind)
}

// announceChoices tells chatID's clients its questions, after the desk moved
// which of them a relay holds. It reads the chat under its writer, so it is
// ordered with every publication of the chat's rows.
func (u *Usecase) announceChoices(chatID string) {
	feed := u.feed.Load()
	if feed == nil || feed.Choices == nil {
		return
	}
	ctx := context.Background()
	err := u.ledger.View(ctx, chatID, func(st ledger.State) error {
		snap, err := u.snapshots.Held(ctx, chatID)
		if err != nil {
			return err
		}
		u.transcript.choices.publish(feed, chatID, snap.Chat.WorkspaceID, transcript.PendingChoicesOf(st), u.answers.AnswerableIDs)
		return nil
	})
	if err != nil {
		slog.Warn("agent: announce choices", "chat_id", chatID, "err", err)
	}
}

// hubFrames carry the recorder's frames on the hub's feed, once
// StartTerminalWaitSweep wires it.
type hubFrames struct{ feed *atomic.Pointer[ChatFeed] }

func (h hubFrames) MessageDelta(chatID, workspaceID, messageID, text, kind string, startedAt time.Time) {
	if feed := h.feed.Load(); feed != nil && feed.MessageDelta != nil {
		feed.MessageDelta(chatID, workspaceID, messageID, text, kind, startedAt)
	}
}

func (h hubFrames) Telemetry(chatID, workspaceID string, report engineagents.Telemetry) {
	if feed := h.feed.Load(); feed != nil && feed.Telemetry != nil {
		feed.Telemetry(chatID, workspaceID, report)
	}
}

// choiceFeed remembers what each chat was last told of its questions, so a
// change that moves nothing a client can see is not announced again.
type choiceFeed struct {
	mu   sync.Mutex
	sent map[string]string
}

func (c *choiceFeed) publish(
	feed *ChatFeed, chatID, wsID string, choices []domain.ActivityChoice,
	answerableIDs func(string, []domain.ActivityChoice) []string,
) {
	if feed.Choices == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sent == nil {
		c.sent = map[string]string{}
	}
	previous, announced := c.sent[chatID]
	if len(choices) == 0 {
		// The clear is owed only to a client that was told of a question.
		if announced {
			delete(c.sent, chatID)
			feed.Choices(chatID, wsID, nil, nil)
		}
		return
	}
	answerable := answerableIDs(chatID, choices)
	fingerprint := choiceFingerprint(choices, answerable)
	if announced && previous == fingerprint {
		return
	}
	c.sent[chatID] = fingerprint
	feed.Choices(chatID, wsID, choices, answerable)
}

// forget drops chatID: the chat is being erased.
func (c *choiceFeed) forget(chatID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sent, chatID)
}

// feedTranscript is the transcript with what each chat's clients were last
// told of its questions: erasing a chat forgets both.
type feedTranscript struct {
	*transcript.Projector
	choices choiceFeed
}

func (t *feedTranscript) Forget(chatID string) {
	t.Projector.Forget(chatID)
	t.choices.forget(chatID)
}

func choiceFingerprint(choices []domain.ActivityChoice, answerable []string) string {
	ids := make([]string, 0, len(choices))
	for _, c := range choices {
		ids = append(ids, c.ID)
	}
	held := slices.Sorted(slices.Values(answerable))
	return strings.Join(ids, ",") + "|" + strings.Join(held, ",")
}
