package chat_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
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

	assert.Equal(t, settledPrompt{chatID, f.chat(t, chatID).WorkspaceID, requestID, false}, <-settled)
	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID))
	f.term.err = nil
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "never forked", requestID, "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptRequestIDConflict)
}
