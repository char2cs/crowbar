package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// numbered is rows as an import hands them over: chat, seq and time set.
func numbered(chatID string, start time.Time, rows ...ledger.Row) []ledger.Row {
	for i := range rows {
		rows[i].ChatID, rows[i].Seq, rows[i].At = chatID, int64(i+1), start.Add(time.Duration(i)*time.Second)
	}
	return rows
}

func TestStore_ImportKeepsSeqAndAtAndRefusesANonEmptyChat(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	var published int
	s.Subscribe(func(string, []ledger.Row, ledger.State) { published++ })
	then := time.Date(2025, 3, 4, 5, 6, 7, 891011000, time.UTC)
	rows := numbered("c1", then, boundS(t, "p"), prompt(t, "r1", "hi"), started(t, "r1"))

	require.NoError(t, s.Import(ctx, "c1", rows))

	got, err := s.Read(ctx, "c1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, rows, got, "seq and time are the import's, not the store clock's")
	assert.Zero(t, published, "an import publishes nothing")
	st, err := s.State(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(3), st.Head())
	assert.True(t, st.HasOpenWork())
	open, err := s.OpenChats(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, open, "the head row is kept with the rows")
	owner, ok, err := s.SessionOwner(ctx, "p", "S")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "c1", owner, "the session index is kept with the rows")

	err = s.Import(ctx, "c1", numbered("c1", then, prompt(t, "r2", "again")))
	require.ErrorIs(t, err, ledgerstore.ErrChatNotEmpty)
	_, _, err = s.Append(ctx, "c2", prompt(t, "r3", "live"))
	require.NoError(t, err)
	require.ErrorIs(t, s.Import(ctx, "c2", numbered("c2", then, prompt(t, "r4", "x"))), ledgerstore.ErrChatNotEmpty)
}

func TestStore_ImportRefusesRowsItCannotKeepAsGiven(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	then := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	gap := numbered("c1", then, prompt(t, "r1", "a"), prompt(t, "r2", "b"))
	gap[1].Seq = 3
	malformed := numbered("c1", then, mustRow(t, ledger.KindTurnStarted, "", ledger.TurnStarted{}))
	foreign := numbered("c2", then, prompt(t, "r1", "a"))

	for name, rows := range map[string][]ledger.Row{"gap": gap, "malformed": malformed, "another chat's": foreign} {
		require.ErrorIs(t, s.Import(ctx, "c1", rows), ledger.ErrMalformedRow, name)
	}
	_, _, err := s.Append(ctx, "c3", boundS(t, "p"))
	require.NoError(t, err)
	claimed := numbered("c1", then, boundS(t, "p"))
	require.ErrorIs(t, s.Import(ctx, "c1", claimed), ledgerstore.ErrSessionClaimed)
	got, err := s.Read(ctx, "c1", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, got, "a refused import writes nothing")
	require.NoError(t, s.Import(ctx, "c1", nil), "nothing to import is no error")
}
