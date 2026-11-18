package ledger_test

import (
	"context"
	"strings"
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

func TestWindow_TakesTheNewestRowsInWholeTurns(t *testing.T) {
	s := newStore(t)
	turns(t, s, 4)

	after, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Rows: 4}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(6), after, "turn 3, which the newest 4 rows cut, comes whole")

	all, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Rows: 12}, nil)
	require.NoError(t, err)
	assert.Zero(t, all, "a window wider than the chat is all of it")
}

func TestWindow_PullsInATurnStillOpenOrNamedInsideIt(t *testing.T) {
	s := newStore(t)
	turns(t, s, 4)

	kept, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Rows: 1}, []string{"T-r2"})
	require.NoError(t, err)
	assert.Equal(t, int64(3), kept, "an open turn's rows come whole, and every row after them")

	// A subagent of turn 2 reports after turn 4 began: cutting turn 2 would orphan it.
	late := mustRow(t, ledger.KindAssistantText, "T-r2", ledger.AssistantText{MessageID: "m", Index: -1, Text: "late"})
	late.Scope = "sub-1"
	_, _, err = s.Append(t.Context(), "c1", prompt(t, "r5", "r5"), started(t, "r5"), late)
	require.NoError(t, err)
	pulled, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Rows: 1}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), pulled)
}

// bulky appends turns r1..rn whose prompt carries size bytes of text.
func bulky(t *testing.T, s *ledgerstore.Store, n, size int) {
	t.Helper()
	text := strings.Repeat("x", size)
	for i := 1; i <= n; i++ {
		id := "r" + string(rune('0'+i))
		_, _, err := s.Append(context.Background(), "c1", prompt(t, id, text), started(t, id), ended(t, id))
		require.NoError(t, err)
	}
}

func TestWindow_ABytesBudgetTakesTheNewestWholeTurnsThatFit(t *testing.T) {
	s := newStore(t)
	bulky(t, s, 5, 1000)

	after, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Bytes: 2500}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(9), after, "turn 4, which the budget cuts, comes whole")

	tiny, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Bytes: 1}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(12), tiny, "the newest turn is always sent, however small the budget")

	all, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Bytes: 1 << 20}, nil)
	require.NoError(t, err)
	assert.Zero(t, all, "a budget past the chat is all of it")
}

func TestWindow_RowsAndBytesBoundTogether(t *testing.T) {
	s := newStore(t)
	bulky(t, s, 5, 10)

	after, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Rows: 4, Bytes: 1 << 20}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(9), after, "the rows bound is the tighter one")
}

func TestWindow_UptoPagesTheTurnsBehindARowWithoutCrossingIt(t *testing.T) {
	s := newStore(t)
	bulky(t, s, 5, 1000)

	after, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Upto: 9, Bytes: 1500}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(6), after, "turn 3 ends at seq 9 and, cut by the budget, comes whole")

	first, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Upto: 3, Bytes: 1 << 20}, nil)
	require.NoError(t, err)
	assert.Zero(t, first, "nothing is older than the first turn")
}

func TestWindow_APreviewCapCountsABulkyRowAtItsPreviewSize(t *testing.T) {
	s := newStore(t)
	text := strings.Repeat("x", 50000)
	for i := 1; i <= 4; i++ {
		id := "r" + string(rune('0'+i))
		_, _, err := s.Append(t.Context(), "c1", prompt(t, id, id), started(t, id),
			mustRow(t, ledger.KindReasoning, "T-"+id, ledger.Reasoning{Text: text}), ended(t, id))
		require.NoError(t, err)
	}

	whole, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Bytes: 6000}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(12), whole, "a 50 KB row overflows the budget: the newest turn alone")

	cut, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Bytes: 6000, PreviewCap: 2000}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(8), cut, "cut to 2 KB the reasoning fits twice over: two turns")
}

func TestRegression_WindowCountsABulkyToolOutputAtItsPreviewSize(t *testing.T) {
	s := newStore(t)
	out := strings.Repeat("x", 50000)
	for i := 1; i <= 4; i++ {
		id := "r" + string(rune('0'+i))
		_, _, err := s.Append(t.Context(), "c1", prompt(t, id, id), started(t, id),
			mustRow(t, ledger.KindToolCall, "T-"+id, ledger.ToolCall{ToolID: id, Phase: ledger.ToolPost, Output: out}), ended(t, id))
		require.NoError(t, err)
	}

	cut, err := s.Window(t.Context(), "c1", ledgerstore.WindowSpec{Bytes: 6000, PreviewCap: 2000}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(8), cut, "a 50 KB output previewed to 2 KB fits twice over: two turns, not one")
}
