package chat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
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
