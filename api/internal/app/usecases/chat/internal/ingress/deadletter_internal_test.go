package ingress

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

func TestDeadLetter_RecordsTheReasonAndTheRunner(t *testing.T) {
	f := newFixture()
	h := hook{runnerID: "runner-1", provider: "p", event: "tool_pre", raw: []byte(`{"x":1}`)}

	require.NoError(t, f.ingress.deadLetter(t.Context(), h, "chat-1", reasonUnknownRunner))

	require.Len(t, f.ledger.letters, 1)
	assert.Equal(t, ledgerstore.DeadLetter{
		ChatID: "chat-1", RunnerID: "runner-1", Provider: "p", Event: "tool_pre",
		Reason: reasonUnknownRunner, Payload: []byte(`{"x":1}`),
	}, f.ledger.letters[0])
}

// Refusals counts the hooks refused to their senders, not those acknowledged
// and kept: an internal session's hooks are routine and say nothing is wrong.
func TestRefusals_CountsTheRefusedNotTheAcknowledged(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	store, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	in := New(Deps{Ledger: store})
	for _, reason := range []string{reasonForeign, reasonNotRecorded, reasonUnknownRunner, reasonWriteFailed} {
		require.NoError(t, in.deadLetter(t.Context(), hook{runnerID: "runner-1"}, "", reason))
	}

	n, err := in.Refusals(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

// A refusal stands whether or not its dead letter could be kept.
func TestDeadLetter_ARefusalWhoseLetterFailsIsStillRefused(t *testing.T) {
	f := newFixture()
	f.ledger.failLetters = errors.New("disk full")

	err := f.ingress.refuse(t.Context(), hook{runnerID: "runner-1"}, "", reasonUnknownRunner, ErrUnknownRunner)

	require.ErrorIs(t, err, ErrUnknownRunner)
	assert.Empty(t, f.ledger.letters)
}
