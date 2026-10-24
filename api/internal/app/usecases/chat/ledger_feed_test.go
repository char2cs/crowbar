package chat_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// newLedger is an in-memory chat ledger.
func newLedger(tb testing.TB) *ledgerstore.Store {
	tb.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(tb, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(tb, err)
	return l
}

// receipt is requestID's prompt as chatID's ledger has folded it.
func receipt(t *testing.T, f testFixture, chatID, requestID string) ledger.Prompt {
	t.Helper()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	p, ok := st.Receipt(requestID)
	require.True(t, ok, "the ledger holds no prompt %s", requestID)
	return p
}

// crowbarPrompts is every prompt Crowbar recorded on chatID, oldest first.
func crowbarPrompts(t *testing.T, f testFixture, chatID string) []ledger.Prompt {
	t.Helper()
	var out []ledger.Prompt
	for _, r := range ledgerRows(t, f, chatID) {
		if msg, err := ledger.DecodeBody[ledger.UserMessage](r); err == nil &&
			r.Kind == ledger.KindUserMessage && msg.Origin == ledger.OriginCrowbar {
			out = append(out, receipt(t, f, chatID, msg.RequestID))
		}
	}
	return out
}

func ledgerRows(t *testing.T, f testFixture, chatID string) []ledger.Row {
	t.Helper()
	rows, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	return rows
}

// plantPrompt records a pending prompt carried by carrier, as a departure that
// never settled it would leave it.
func plantPrompt(t *testing.T, f testFixture, chatID, carrier string) string {
	t.Helper()
	requestID := uuid.NewString()
	row, err := ledger.NewRow(ledger.KindUserMessage, uuid.NewString(), "", ledger.UserMessage{
		RequestID: requestID, Text: "can you call some subagents?", Origin: ledger.OriginCrowbar,
		Delivery: ledger.DeliveryRestart, RunnerID: carrier,
	})
	require.NoError(t, err)
	_, _, err = f.ledger.Append(f.ctx, chatID, row)
	require.NoError(t, err)
	return requestID
}

type settledPrompt struct {
	chatID, workspaceID, requestID string
	consumed                       bool
}

// A prompt whose replacement CLI never started is settled, and the chat's
// clients hear it by request id: the composer keeps the text, and a resend of
// that request can never deliver it late.
func TestLedgerFeed_PublishesAPromptSettledWithoutATurn(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	settled := make(chan settledPrompt, 1)
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{
		PromptSettled: func(chatID, workspaceID, requestID string, consumed bool) {
			settled <- settledPrompt{chatID, workspaceID, requestID, consumed}
		},
	})
	f.term.err = errors.New("replacement create failed")
	requestID := uuid.NewString()

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "never forked", requestID, "", nil)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown,
		"a failed create may follow a fork, so the client is not told it never ran")

	deadline, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	select {
	case got := <-settled:
		assert.Equal(t, settledPrompt{chatID, f.chat(t, chatID).WorkspaceID, requestID, false}, got)
	case <-deadline.Done():
		t.Fatal("no prompt_settled was published")
	}
	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID))
	f.term.err = nil
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "never forked", requestID, "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptRequestIDConflict)
}

// A chat's first commit reaches a subscriber that reads the chat's snapshot.
// Its facts are installed first, so no subscriber reads the ledger under the
// writer still holding it.
func TestLedgerFeed_AChatsFirstCommitCompletesAndItsSnapshotCarriesTheConversation(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "codex", "")
	require.NoError(t, err)
	require.Empty(t, ledgerRows(t, f, chatID), "precondition: nothing has touched the chat's ledger")
	settled := make(chan settledPrompt, 1)
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{
		PromptSettled: func(chatID, workspaceID, requestID string, consumed bool) {
			settled <- settledPrompt{chatID, workspaceID, requestID, consumed}
		},
	})
	bound, err := ledger.Assign(ledger.State{}, "codex", "thread-1")
	require.NoError(t, err)
	asked, err := ledger.NewRow(ledger.KindUserMessage, "turn-1", "", ledger.UserMessage{
		RequestID: "req-1", Text: "hi", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart,
	})
	require.NoError(t, err)
	gaveUp, err := ledger.NewRow(ledger.KindPromptSettled, "", "", ledger.PromptSettled{
		RequestID: "req-1", Reason: ledger.ReasonDeliveryFailed,
	})
	require.NoError(t, err)

	appendWithin(t, f, chatID, bound, asked, gaveUp)
	deadline, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	select {
	case got := <-settled:
		assert.Equal(t, "req-1", got.requestID)
	case <-deadline.Done():
		t.Fatal("no prompt_settled was published")
	}

	snap, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	c, ok := snap.Conversation()
	require.True(t, ok, "the snapshot carries the conversation the commit bound")
	assert.Equal(t, "thread-1", c.SessionID)
}

// A commit for a chat the snapshot owner cannot read (here: none exists) still
// completes: its publication never reads the ledger under the chat's writer.
func TestLedgerFeed_ACommitForAChatThatCannotBeReadCompletes(t *testing.T) {
	f := newFixture(t)
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{
		PromptSettled: func(string, string, string, bool) {},
	})
	bound, err := ledger.Assign(ledger.State{}, "codex", "thread-1")
	require.NoError(t, err)
	asked, err := ledger.NewRow(ledger.KindUserMessage, "turn-1", "", ledger.UserMessage{
		RequestID: "req-1", Text: "hi", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart,
	})
	require.NoError(t, err)
	gaveUp, err := ledger.NewRow(ledger.KindPromptSettled, "", "", ledger.PromptSettled{
		RequestID: "req-1", Reason: ledger.ReasonDeliveryFailed,
	})
	require.NoError(t, err)

	appendWithin(t, f, uuid.NewString(), bound, asked, gaveUp)
}

// appendWithin commits rows to chatID, failing the test if the commit does not
// return: a subscriber reading the ledger under its writer never would.
func appendWithin(t *testing.T, f testFixture, chatID string, rows ...ledger.Row) {
	t.Helper()
	committed := make(chan error, 1)
	go func() {
		_, _, err := f.ledger.Append(f.ctx, chatID, rows...)
		committed <- err
	}()
	deadline, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	select {
	case err := <-committed:
		require.NoError(t, err)
	case <-deadline.Done():
		t.Fatal("the commit deadlocked on its own subscribers")
	}
}

// The provider's idle after its turn_stop names the turn that stop closed: it
// is written late and announces no second turn_stopped.
func TestLedgerFeed_AnIdleAfterTheTurnStopAnnouncesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook := func(event string, payload map[string]any) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", event, mustJSON(t, payload)))
		f.wait()
	}
	hook("user_prompt", map[string]any{"prompt": "one", "prompt_id": "A"})
	hook("turn_stop", map[string]any{"last_assistant_message": "", "prompt_id": "A"})
	written := len(ledgerRows(t, f, chatID))
	f.bc.reset()

	hook("idle", map[string]any{"message": "Claude is waiting for your input"})

	assert.Empty(t, f.bcKinds(t))
	rows := ledgerRows(t, f, chatID)
	require.Len(t, rows, written+1, "the idle is written")
	assert.Equal(t, ledger.KindTurnEnded, rows[written].Kind)
}

// A stop naming a turn that already ended is written late, and announces no
// turn edge: a client clears the running turn's live stream on one.
func TestLedgerFeed_ALateTurnStopAnnouncesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook := func(event string, payload map[string]any) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", event, mustJSON(t, payload)))
		f.wait()
	}
	hook("user_prompt", map[string]any{"prompt": "one", "prompt_id": "A"})
	hook("turn_stop", map[string]any{"last_assistant_message": "", "prompt_id": "A"})
	hook("user_prompt", map[string]any{"prompt": "two", "prompt_id": "B"})
	f.bc.reset()

	hook("turn_stop", map[string]any{"last_assistant_message": "", "prompt_id": "A"})

	assert.Empty(t, f.bcKinds(t))
	assert.True(t, f.busy(t, chatID), "B still runs")
	rows := ledgerRows(t, f, chatID)
	assert.Equal(t, ledger.KindTurnEnded, rows[len(rows)-1].Kind, "the late stop is written")
}

// A subagent's stop closes the questions it held, and the chat's clients are
// told: no card is left without its asker.
func TestLedgerFeed_ASubagentsStopClearsTheQuestionsItHeld(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	var mu sync.Mutex
	var frames []int
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{
		Choices: func(_, _ string, choices []domain.ActivityChoice, _ []string) {
			mu.Lock()
			defer mu.Unlock()
			frames = append(frames, len(choices))
		},
	})
	row := func(kind ledger.Kind, scope string, body any) ledger.Row {
		r, err := ledger.NewRow(kind, "T1", scope, body)
		require.NoError(t, err)
		return r
	}
	appendWithin(t, f, chatID,
		row(ledger.KindTurnStarted, "", ledger.TurnStarted{RunnerID: runnerID}),
		row(ledger.KindSubagent, "A", ledger.Subagent{Phase: ledger.SubagentStart}),
		row(ledger.KindAsk, "A", ledger.Ask{AskID: "choice-1", Event: "permission", Kind: domain.ChoiceKindPermission, Answerable: true}),
	)
	appendWithin(t, f, chatID, row(ledger.KindSubagent, "A", ledger.Subagent{Phase: ledger.SubagentStop}))
	f.wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{1, 0}, frames, "the ask, then its close")
}
