package ledger_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
)

func TestStore_DeadLettersKeepThePayloadTruncatedAt64KiB(t *testing.T) {
	db := openDB(t, filepath.Join(t.TempDir(), ledgerstore.FileName))
	s, err := ledgerstore.Open(db, clock)
	require.NoError(t, err)
	payload := make([]byte, 70000)

	require.NoError(t, s.DeadLetter(t.Context(), ledgerstore.DeadLetter{
		RunnerID: "r1", Provider: "p", Event: "tool_pre", Reason: "unknown_runner", Payload: payload,
	}))

	var stored struct {
		Length    int
		Truncated bool
		ChatID    string
	}
	require.NoError(t, db.Raw(`SELECT length(payload) AS length, truncated, chat_id FROM ledger_dead_letters`).
		Scan(&stored).Error)
	assert.Equal(t, 65536, stored.Length)
	assert.True(t, stored.Truncated)
	assert.Empty(t, stored.ChatID, "a runner Crowbar does not know names no chat")
}

// Dead letters are counted by why they were kept, and go with their chat.
func TestStore_DeadLettersAreCountedByReasonAndForgottenWithTheirChat(t *testing.T) {
	s, err := ledgerstore.Open(openDB(t, filepath.Join(t.TempDir(), ledgerstore.FileName)), clock)
	require.NoError(t, err)
	for _, d := range []ledgerstore.DeadLetter{{ChatID: "c1", Reason: "a"}, {ChatID: "c1", Reason: "b"}, {ChatID: "c2", Reason: "a"}} {
		require.NoError(t, s.DeadLetter(t.Context(), d))
	}

	require.NoError(t, s.Forget(t.Context(), "c1"))

	count, err := s.DeadLetterCount(t.Context(), "a", "b")
	require.NoError(t, err)
	assert.Equal(t, 1, count, "c2's alone are left")
	count, err = s.DeadLetterCount(t.Context(), "b")
	require.NoError(t, err)
	assert.Zero(t, count)
}

// A dead letter outlives no retention, whatever chat it names or why it was
// kept: the next open purges the ones older than it and keeps the rest.
func TestStore_DeadLettersOlderThanTheRetentionArePurgedAtOpen(t *testing.T) {
	db := openDB(t, filepath.Join(t.TempDir(), ledgerstore.FileName))
	now := fixedNow
	at := func() time.Time { return now }
	s, err := ledgerstore.Open(db, at)
	require.NoError(t, err)
	for _, d := range []ledgerstore.DeadLetter{
		{Reason: "unknown_runner"}, {ChatID: "c1", Reason: "foreign_conversation"},
	} {
		require.NoError(t, s.DeadLetter(t.Context(), d))
	}
	now = now.Add(ledgerstore.DeadLetterRetention / 2)
	require.NoError(t, s.DeadLetter(t.Context(), ledgerstore.DeadLetter{Reason: "unknown_runner"}))

	now = fixedNow.Add(ledgerstore.DeadLetterRetention + time.Second)
	s, err = ledgerstore.Open(db, at)
	require.NoError(t, err)

	count, err := s.DeadLetterCount(t.Context(), "unknown_runner", "foreign_conversation")
	require.NoError(t, err)
	assert.Equal(t, 1, count, "only the letter younger than the retention is kept")
}
