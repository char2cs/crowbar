package ledger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// relayBound is a session "S" bound with relay id "R1".
func relayBound(t *testing.T, cause, from string) ledger.Row {
	t.Helper()
	return mustRow(t, ledger.KindSessionBound, "", ledger.SessionBound{
		Provider: "p", SessionID: "S", Cause: cause, From: from, RelayID: "R1",
	})
}

func TestStore_ARelayIdNamesTheChatWhoseBindingHoldsIt(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", relayBound(t, ledger.CauseAnnounced, ""))
	require.NoError(t, err)

	owner, ok, err := s.RelayOwner(ctx, "R1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "c1", owner)

	_, ok, err = s.RelayOwner(ctx, "R-unknown")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestStore_AMovedSessionTakesItsRelayIdToTheChatThatTookIt(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", relayBound(t, ledger.CauseAnnounced, ""))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c1", gaveUpS(t, "p", "c2"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c2", relayBound(t, ledger.CauseMoved, "c1"))
	require.NoError(t, err)

	owner, _, err := s.RelayOwner(ctx, "R1")
	require.NoError(t, err)
	assert.Equal(t, "c2", owner)
}

func TestStore_AnotherChatCannotTakeARelayIdWithoutAMove(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", relayBound(t, ledger.CauseAnnounced, ""))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c2", mustRow(t, ledger.KindSessionBound, "", ledger.SessionBound{
		Provider: "p", SessionID: "other", Cause: ledger.CauseAnnounced, RelayID: "R1",
	}))
	require.NoError(t, err)

	owner, _, err := s.RelayOwner(ctx, "R1")
	require.NoError(t, err)
	assert.Equal(t, "c1", owner)
}

func TestStore_AForgottenChatHoldsNoRelayId(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", relayBound(t, ledger.CauseAnnounced, ""))
	require.NoError(t, err)
	require.NoError(t, s.Forget(ctx, "c1"))

	_, ok, err := s.RelayOwner(ctx, "R1")
	require.NoError(t, err)
	assert.False(t, ok)
}
