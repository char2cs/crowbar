package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func moveRow(t *testing.T, m ledger.Move) ledger.Row {
	t.Helper()
	return row(t, ledger.KindMove, "", m)
}

func TestMove_ARequestWaitsForTheRunningTurnToEnd(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "r1", RunnerID: "run-1"}),
		moveRow(t, ledger.Move{Phase: ledger.MoveRequested, RequestID: "m1", To: "ws-b", Label: "b2"}),
	))
	pending, ok := st.PendingMove()
	require.True(t, ok)
	assert.Equal(t, "ws-b", pending.To)
	_, ready := st.MoveReady()
	assert.False(t, ready, "a move never runs over a running turn")
	assert.True(t, st.HasOpenWork(), "a pending move is work the boot pass must see")

	st.Apply(withSeq(row(t, ledger.KindTurnEnded, "T1", ledger.TurnEnded{Status: "done"}), 5))
	got, ready := st.MoveReady()
	require.True(t, ready)
	assert.Equal(t, "m1", got.RequestID)
}

func TestMove_DoneLeavesTheOldFolderAndOwesTheNextPromptANote(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		moveRow(t, ledger.Move{Phase: ledger.MoveRequested, RequestID: "m1", To: "ws-b"}),
		moveRow(t, ledger.Move{
			Phase: ledger.MoveDone, RequestID: "m1", From: "ws-a", To: "ws-b",
			FromDir: "/home/p", ToDir: "/wt/b2", Branch: "b2", Base: "abc1234",
		}),
	))
	_, pending := st.PendingMove()
	assert.False(t, pending)
	assert.Equal(t, []string{"/home/p"}, st.LeftDirs())
	note := st.MoveNote()
	assert.Contains(t, note, "/wt/b2")
	assert.Contains(t, note, "b2")
	assert.Contains(t, note, "abc1234")
	assert.Contains(t, note, "/home/p")
	assert.Contains(t, note, "not brought over")

	st.Apply(withSeq(crowbarPrompt(t, "r1", "a", "T1"), 3))
	st.Apply(withSeq(row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "r1", RunnerID: "run-1"}), 4))
	assert.Empty(t, st.MoveNote(), "the note rides one delivered prompt")
	assert.Equal(t, []string{"/home/p"}, st.LeftDirs(), "the folder stays guarded")
}

func TestMove_ComingBackToAFolderStopsGuardingIt(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		moveRow(t, ledger.Move{Phase: ledger.MoveDone, RequestID: "m1", FromDir: "/a", ToDir: "/b", To: "ws-b"}),
		moveRow(t, ledger.Move{Phase: ledger.MoveDone, RequestID: "m2", FromDir: "/b", ToDir: "/a", To: "ws-a"}),
	))
	assert.Equal(t, []string{"/b"}, st.LeftDirs())
}

func TestMove_ANoteForAnotherProjectNamesItsRepository(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		moveRow(t, ledger.Move{Phase: ledger.MoveDone, RequestID: "m1", FromDir: "/a", ToDir: "/beta", Repo: "beta"}),
	))
	assert.Contains(t, st.MoveNote(), "repository beta")
}

func TestMove_ACancelOrFailureClosesTheRequest(t *testing.T) {
	for _, phase := range []string{ledger.MoveCancelled, ledger.MoveFailed} {
		st := ledger.Fold("c1", sequenced(t,
			moveRow(t, ledger.Move{Phase: ledger.MoveRequested, RequestID: "m1", To: "ws-b"}),
			moveRow(t, ledger.Move{Phase: phase, RequestID: "m1", Reason: "why"}),
		))
		_, pending := st.PendingMove()
		assert.False(t, pending, phase)
		assert.Empty(t, st.LeftDirs(), phase)
		assert.Empty(t, st.MoveNote(), phase)
	}
}

func TestMove_CheckRefusesAnUnknownPhaseOrAMissingRequest(t *testing.T) {
	require.ErrorIs(t, ledger.Check(ledger.State{}, moveRow(t, ledger.Move{Phase: "maybe", RequestID: "m"})), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, moveRow(t, ledger.Move{Phase: ledger.MoveRequested})), ledger.ErrMalformedRow)
	assert.NoError(t, ledger.Check(ledger.State{}, moveRow(t, ledger.Move{Phase: ledger.MoveRequested, RequestID: "m", To: "w"})))
}

func withSeq(r ledger.Row, seq int64) ledger.Row {
	r.ChatID, r.Seq = "c1", seq
	return r
}

// An agent's branch-out waits for the person's word; it holds no turn, so it survives restarts.
func TestMove_AnAgentsBranchOutWaitsForThePersonsConsent(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		moveRow(t, ledger.Move{Phase: ledger.MoveRequested, RequestID: "m1", BranchOut: true, By: "agent", AwaitsConsent: true}),
	))
	pending, ok := st.PendingMove()
	require.True(t, ok)
	assert.True(t, pending.AwaitsConsent)
	_, ready := st.MoveReady()
	assert.False(t, ready, "nothing the person has not allowed runs")

	st.Apply(withSeq(moveRow(t, ledger.Move{Phase: ledger.MoveApproved, RequestID: "m1"}), 2))
	got, ready := st.MoveReady()
	require.True(t, ready)
	assert.False(t, got.AwaitsConsent)
	assert.True(t, got.BranchOut, "allowing keeps what was asked")
}
