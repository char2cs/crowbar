package ledger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func ended(t *testing.T, requestID string) ledger.Row {
	t.Helper()
	return mustRow(t, ledger.KindTurnEnded, "T-"+requestID, ledger.TurnEnded{Status: "done"})
}

// turns appends n whole turns r1..rn (prompt, start, end: seqs 3i-2..3i).
func turns(t *testing.T, s *ledgerstore.Store, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		id := "r" + string(rune('0'+i))
		_, _, err := s.Append(context.Background(), "c1", prompt(t, id, id), started(t, id), ended(t, id))
		require.NoError(t, err)
	}
}

func TestWindowStart_TakesTheNewestTurnsWhole(t *testing.T) {
	s := newStore(t)
	turns(t, s, 4)

	after, err := s.WindowStart(t.Context(), "c1", 2, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(6), after, "turn 3's prompt is the window's first row")

	all, err := s.WindowStart(t.Context(), "c1", 9, nil)
	require.NoError(t, err)
	assert.Zero(t, all, "a window wider than the chat is all of it")
}

func TestWindowStart_PullsInATurnStillOpenOrNamedInsideIt(t *testing.T) {
	s := newStore(t)
	turns(t, s, 4)

	kept, err := s.WindowStart(t.Context(), "c1", 1, []string{"T-r2"})
	require.NoError(t, err)
	assert.Equal(t, int64(3), kept, "an open turn's rows come whole, and every row after them")

	// A subagent of turn 2 reports after turn 4 began: cutting turn 2 would orphan it.
	late := mustRow(t, ledger.KindAssistantText, "T-r2", ledger.AssistantText{MessageID: "m", Index: -1, Text: "late"})
	late.Scope = "sub-1"
	_, _, err = s.Append(t.Context(), "c1", prompt(t, "r5", "r5"), started(t, "r5"), late)
	require.NoError(t, err)
	pulled, err := s.WindowStart(t.Context(), "c1", 1, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), pulled)
}
