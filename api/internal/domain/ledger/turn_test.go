package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func turnStarted(t *testing.T, turnID, runnerID string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindTurnStarted, turnID, ledger.TurnStarted{RunnerID: runnerID})
}

func turnEnded(t *testing.T, turnID string, body ledger.TurnEnded) ledger.Row {
	t.Helper()
	return row(t, ledger.KindTurnEnded, turnID, body)
}

func TestTurnStatus_Next(t *testing.T) {
	all := []ledger.TurnStatus{
		ledger.TurnStatusPending, ledger.TurnStatusRunning, ledger.TurnStatusDone, ledger.TurnStatusFailed, ledger.TurnStatusInterrupted,
	}
	allowed := map[[2]ledger.TurnStatus]bool{
		{ledger.TurnStatusPending, ledger.TurnStatusRunning}:     true,
		{ledger.TurnStatusRunning, ledger.TurnStatusDone}:        true,
		{ledger.TurnStatusRunning, ledger.TurnStatusFailed}:      true,
		{ledger.TurnStatusRunning, ledger.TurnStatusInterrupted}: true,
	}
	for _, from := range all {
		for _, to := range all {
			got, ok := from.Next(to)
			if allowed[[2]ledger.TurnStatus{from, to}] {
				assert.True(t, ok, "%s → %s", from, to)
				assert.Equal(t, to, got)
				continue
			}
			assert.False(t, ok, "%s → %s", from, to)
			assert.Equal(t, from, got, "a refused transition stays where it was")
		}
	}
}

func TestFold_TurnLifecycle(t *testing.T) {
	rows := sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "r1", RunnerID: "run-1", ProviderTurnID: "p1"}),
	)
	st := ledger.Fold("c1", rows)
	running, ok := st.RunningTurn()
	require.True(t, ok)
	assert.Equal(t, ledger.Turn{
		ID: "T1", ProviderTurnID: "p1", RunnerID: "run-1", RequestID: "r1",
		Status: ledger.TurnStatusRunning, StartedAt: rows[1].At,
	}, running)
	assert.True(t, st.Busy())
	assert.True(t, st.HasOpenWork(), "a running turn is unfinished work")

	ended := turnEnded(t, "T1", ledger.TurnEnded{Status: "done"})
	ended.Seq = 3
	st.Apply(ended)
	_, ok = st.RunningTurn()
	assert.False(t, ok)
	assert.False(t, st.Busy())
	assert.False(t, st.HasOpenWork())

	late := turnEnded(t, "T1", ledger.TurnEnded{Status: "interrupted", Reason: ledger.ReasonStopped})
	late.Seq = 4
	before := st.Clone()
	st.Apply(late)
	assert.Equal(t, before.Busy(), st.Busy(), "a late close changes nothing")
	_, ok = st.RunningTurn()
	assert.False(t, ok)

	for i, r := range sequenced(t,
		turnStarted(t, "T2", "run-1"),
		row(t, ledger.KindTurnFailed, "T2", ledger.TurnFailed{Reason: "usage_limit", Detail: "limit reached"}),
	) {
		r.Seq = int64(5 + i)
		st.Apply(r)
	}
	_, ok = st.RunningTurn()
	assert.False(t, ok, "T2 failed")
	assert.False(t, st.Busy())
}

// A close naming a turn that is not the running one leaves it running, and
// so does a close of the running turn into a status the diagram refuses.
func TestFold_ACloseOfAnotherTurnKeepsTheRunningOne(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStarted(t, "T1", "run-1"),
		turnEnded(t, "T9", ledger.TurnEnded{Status: "done"}),
		row(t, ledger.KindTurnFailed, "T9", ledger.TurnFailed{Reason: "overloaded"}),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "running"}),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "failed"}),
	))
	running, ok := st.RunningTurn()
	require.True(t, ok)
	assert.Equal(t, "T1", running.ID)
	assert.Equal(t, ledger.TurnStatusRunning, running.Status)

	st = ledger.Fold("c1", sequenced(t,
		turnStarted(t, "T1", "run-1"),
		func() ledger.Row {
			r, err := ledger.NewRow(ledger.KindTurnEnded, "T1", "sub-1", ledger.TurnEnded{Status: "done"})
			require.NoError(t, err)
			return r
		}(),
	))
	_, ok = st.RunningTurn()
	assert.True(t, ok, "a subagent scope's close is not the main turn's")
}

// A turn_started over a running turn, or naming a closed one, is late: the
// running turn keeps running, a closed one never reopens, and the prompt it
// names stays pending.
func TestFold_ATurnStartedOverAnOpenTurnIsLate(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStarted(t, "T1", "run-1"),
		crowbarPrompt(t, "r2", "b", "T2"),
		row(t, ledger.KindTurnStarted, "T2", ledger.TurnStarted{RequestID: "r2", RunnerID: "run-1"}),
	))
	running, ok := st.RunningTurn()
	require.True(t, ok)
	assert.Equal(t, "T1", running.ID)
	assert.Len(t, st.PendingPrompts(), 1, "a refused start delivers nothing")

	st = ledger.Fold("c1", sequenced(t,
		turnStarted(t, "T1", "run-1"),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "done"}),
		turnStarted(t, "T1", "run-1"),
	))
	_, ok = st.RunningTurn()
	assert.False(t, ok, "a closed turn never reopens")
}

// An end that closes nothing is still a row: it names the closed turn it is
// about, or a turn the ledger never ran, and never the running one.
func TestEndTurnOf_AnEndThatClosesNothingIsALateRow(t *testing.T) {
	running := ledger.Fold("c1", sequenced(t,
		started(t, "r1"),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RunnerID: "r1", ProviderTurnID: "p1"}),
	))
	r, closes, err := ledger.EndTurnOf(running, "r1", "p1", ledger.TurnEnded{Status: "done", AsyncWork: 1})
	require.NoError(t, err)
	assert.True(t, closes)
	assert.Equal(t, "T1", r.TurnID)
	body, err := ledger.DecodeBody[ledger.TurnEnded](r)
	require.NoError(t, err)
	assert.Equal(t, ledger.TurnEnded{Status: "done", RunnerID: "r1", AsyncWork: 1}, body)

	for name, by := range map[string][2]string{
		"another runner's end":       {"r0", ""},
		"an end naming another turn": {"r1", "p0"},
	} {
		r, closes, err := ledger.EndTurnOf(running, by[0], by[1], ledger.TurnEnded{Status: "done"})
		require.NoError(t, err, name)
		assert.False(t, closes, name)
		assert.NotEqual(t, "T1", r.TurnID, "%s never names the running turn", name)
		require.NoError(t, ledger.Check(running, r), name)
		after := running.Clone()
		r.Seq = 9
		after.Apply(r)
		_, still := after.RunningTurn()
		assert.True(t, still, name)
	}

	closed := running.Clone()
	end := turnEnded(t, "T1", ledger.TurnEnded{Status: "done", RunnerID: "r1"})
	end.Seq = 3
	closed.Apply(end)
	r, closes, err = ledger.EndTurnOf(closed, "r1", "", ledger.TurnEnded{Status: "done"})
	require.NoError(t, err)
	assert.False(t, closes)
	assert.Equal(t, "T1", r.TurnID, "a late end names the closed turn it is about")

	r, closes, err = ledger.FailTurnOf(ledger.Fold("c1", nil), "r1", "", ledger.TurnFailed{Reason: "overloaded"})
	require.NoError(t, err)
	assert.False(t, closes)
	require.NoError(t, ledger.Check(ledger.State{}, r), "a late failure with no turn to name gets one")

	r, closes, err = ledger.FailTurnOf(running, "r1", "p1", ledger.TurnFailed{Reason: "overloaded"})
	require.NoError(t, err)
	assert.True(t, closes)
	assert.Equal(t, "T1", r.TurnID)
}

// The background-work clause reads the live runner's last end, late or not:
// a late restatement clears it, and a dead runner's late end changes nothing.
func TestFold_ALateEndRestatesTheLiveRunnersBackgroundWork(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		started(t, "r1"),
		turnStarted(t, "T1", "r1"),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "done", RunnerID: "r1", AsyncWork: 2}),
	))
	require.True(t, st.Busy())

	apply := func(runnerID string, seq int64) {
		r, closes, err := ledger.EndTurnOf(st, runnerID, "", ledger.TurnEnded{Status: "done"})
		require.NoError(t, err)
		require.False(t, closes)
		r.Seq = seq
		st.Apply(r)
	}
	apply("r0", 4)
	assert.True(t, st.Busy(), "another runner's late end restates nothing of r1's")
	apply("r1", 5)
	assert.False(t, st.Busy(), "the live runner's late end restating no work clears it")
	_, running := st.RunningTurn()
	assert.False(t, running)
}

func TestFold_AsyncWorkKeepsTheChatBusyOnlyWhileARunnerLives(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		started(t, "r1"),
		turnStarted(t, "T1", "r1"),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "done", RunnerID: "r1", AsyncWork: 1}),
	))
	_, running := st.RunningTurn()
	assert.False(t, running)
	assert.True(t, st.Busy(), "background work the turn left keeps the chat busy")

	end := exited(t, "r1", "exited", 0)
	end.Seq = 4
	st.Apply(end)
	assert.False(t, st.Busy(), "no background work outlives the runner that reported it")

	again := started(t, "r2")
	again.Seq = 5
	st.Apply(again)
	assert.False(t, st.Busy(), "a new runner carries none of the old one's background work")
}

func TestEndTurn_ClosesOnlyARunningTurn(t *testing.T) {
	idle := ledger.Fold("c1", nil)
	rows, err := ledger.EndTurn(idle, ledger.TurnStatusDone, ledger.ReasonProviderIdle, 0, "")
	require.NoError(t, err)
	assert.Empty(t, rows)

	st := ledger.Fold("c1", sequenced(t, turnStarted(t, "T1", "run-1")))
	rows, err = ledger.EndTurn(st, ledger.TurnStatusInterrupted, ledger.ReasonStopped, 3, "high")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, ledger.KindTurnEnded, rows[0].Kind)
	assert.Equal(t, "T1", rows[0].TurnID)
	body, err := ledger.DecodeBody[ledger.TurnEnded](rows[0])
	require.NoError(t, err)
	assert.Equal(t, ledger.TurnEnded{
		Status: "interrupted", Reason: "stopped", RunnerID: "run-1", AsyncWork: 3, Effort: "high",
	}, body)

	r, closes, err := ledger.FailTurnOf(st, "run-1", "", ledger.TurnFailed{Reason: "usage_limit", Detail: "limit reached"})
	require.NoError(t, err)
	assert.True(t, closes)
	failed, err := ledger.DecodeBody[ledger.TurnFailed](r)
	require.NoError(t, err)
	assert.Equal(t, ledger.TurnFailed{Reason: "usage_limit", Detail: "limit reached"}, failed)
	assert.Equal(t, "T1", r.TurnID)
}

func TestTurn_NamesAnEventOnlyWhenNeitherIDDisagrees(t *testing.T) {
	named := ledger.Turn{ProviderTurnID: "A"}
	assert.True(t, named.Names("A"))
	assert.True(t, named.Names(""), "an event with no turn id is about the running turn")
	assert.False(t, named.Names("B"), "a stop naming another turn is late")
	assert.True(t, ledger.Turn{}.Names("B"), "a turn started without an id matches any")
}

// A session whose CLI writes it only once a turn completes has turned only
// then; one written at turn start has turned as the turn starts.
func TestFold_ASessionPersistedAtTurnEndTurnsOnlyWhenItsTurnIsDone(t *testing.T) {
	onEnd := func(turnID string) ledger.Row {
		return row(t, ledger.KindTurnStarted, turnID, ledger.TurnStarted{
			RunnerID: "run-1", Provider: "p1", SessionID: "S1", Persists: ledger.PersistsTurnEnd,
		})
	}
	st := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, ""), onEnd("T1")))
	c, _ := st.Conversation("p1")
	assert.False(t, c.Turned, "the turn has not completed")

	failed := ledger.Fold("c1", sequenced(t,
		bound(t, "p1", "S1", ledger.CauseAssigned, ""), onEnd("T1"),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "interrupted", Reason: ledger.ReasonStopped}),
		onEnd("T2"),
		row(t, ledger.KindTurnFailed, "T2", ledger.TurnFailed{Reason: "overloaded"}),
	))
	c, _ = failed.Conversation("p1")
	assert.False(t, c.Turned, "an interrupted or failed turn never completed")

	done := ledger.Fold("c1", sequenced(t,
		bound(t, "p1", "S1", ledger.CauseAssigned, ""), onEnd("T1"),
		turnEnded(t, "T1", ledger.TurnEnded{Status: "done"}),
	))
	c, _ = done.Conversation("p1")
	assert.True(t, c.Turned)

	onStart := ledger.Fold("c1", sequenced(t,
		bound(t, "p1", "S1", ledger.CauseAssigned, ""),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{
			RunnerID: "run-1", Provider: "p1", SessionID: "S1", Persists: ledger.PersistsTurnStart,
		}),
	))
	c, _ = onStart.Conversation("p1")
	assert.True(t, c.Turned)
}

func TestCheck_TurnEndRowsNameTheirTurn(t *testing.T) {
	for name, r := range map[string]ledger.Row{
		"ended without a turn":    turnEnded(t, "", ledger.TurnEnded{Status: "done"}),
		"ended without a status":  turnEnded(t, "T1", ledger.TurnEnded{}),
		"ended as failed":         turnEnded(t, "T1", ledger.TurnEnded{Status: "failed"}),
		"failed without a turn":   row(t, ledger.KindTurnFailed, "", ledger.TurnFailed{Reason: "overloaded"}),
		"failed without a reason": row(t, ledger.KindTurnFailed, "T1", ledger.TurnFailed{}),
	} {
		require.ErrorIs(t, ledger.Check(ledger.State{}, r), ledger.ErrMalformedRow, name)
	}
	require.NoError(t, ledger.Check(ledger.State{}, turnEnded(t, "T1", ledger.TurnEnded{Status: "interrupted"})))
	require.NoError(t, ledger.Check(ledger.State{}, row(t, ledger.KindTurnFailed, "T1", ledger.TurnFailed{Reason: "x"})))
	assert.NotPanics(t, func() {
		var st ledger.State
		st.Apply(ledger.Row{Seq: 1, Kind: ledger.KindTurnEnded, Body: []byte("{")})
		st.Apply(ledger.Row{Seq: 2, Kind: ledger.KindTurnFailed, Body: []byte("{")})
	})
}

// A runner's exit ends the turn it was running, in the same rows; the exit
// of a runner that is not the turn's leaves the turn alone.
func TestExit_EndsTheTurnItsRunnerWasOn(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t, started(t, "r1"), turnStarted(t, "T1", "r1")))

	rows, err := ledger.Exit(st, ledger.RunnerExited{RunnerID: "r1", Reason: "exited"}, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindRunnerExited}, []ledger.Kind{rows[0].Kind, rows[1].Kind})
	body, err := ledger.DecodeBody[ledger.TurnEnded](rows[0])
	require.NoError(t, err)
	assert.Equal(t, ledger.TurnEnded{Status: "interrupted", Reason: ledger.ReasonRunnerExited, RunnerID: "r1"}, body)
	for i := range rows {
		rows[i].Seq = int64(3 + i)
		st.Apply(rows[i])
	}
	assert.False(t, st.Busy())

	other := ledger.Fold("c1", sequenced(t, started(t, "r2"), turnStarted(t, "T1", "r2")))
	rows, err = ledger.Exit(other, ledger.RunnerExited{RunnerID: "r1", Reason: "exited"}, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a late exit closes no turn")
}

func TestStop_InterruptsTheTurnAndSettlesEveryPendingPrompt(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "r1", RunnerID: "run-1"}),
		crowbarPrompt(t, "r2", "b", "T2"),
	))
	rows, err := ledger.Stop(st)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	ended, err := ledger.DecodeBody[ledger.TurnEnded](rows[0])
	require.NoError(t, err)
	assert.Equal(t, ledger.TurnEnded{Status: "interrupted", Reason: "stopped", RunnerID: "run-1"}, ended)
	assert.Equal(t, []ledger.PromptSettled{{RequestID: "r2", Reason: "stopped"}}, settled(t, rows[1:]))

	rows, err = ledger.Stop(ledger.Fold("c1", nil))
	require.NoError(t, err)
	assert.Empty(t, rows, "an idle chat has nothing to stop")
}

// Leaving a session and exiting are decided from the same fold, so whichever
// commits second finds nothing left to close.
func TestLeave_AndExitCloseTheTurnOnce(t *testing.T) {
	base := func() ledger.State {
		return ledger.Fold("c1", sequenced(t,
			started(t, "r1"),
			crowbarPrompt(t, "p1", "a", "T1"),
			row(t, ledger.KindUserMessage, "T2", ledger.UserMessage{
				RequestID: "p2", Text: "b", Origin: ledger.OriginCrowbar, RunnerID: "r1",
			}),
			row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "p1", RunnerID: "r1"}),
		))
	}
	apply := func(st *ledger.State, rows []ledger.Row) {
		for _, r := range rows {
			r.Seq = st.Head() + 1
			st.Apply(r)
		}
	}
	kinds := func(rows []ledger.Row) []ledger.Kind {
		out := make([]ledger.Kind, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.Kind)
		}
		return out
	}

	st := base()
	left, err := ledger.Leave(st, "r1")
	require.NoError(t, err)
	assert.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindPromptSettled}, kinds(left))
	apply(&st, left)
	exit, err := ledger.Exit(st, ledger.RunnerExited{RunnerID: "r1", Reason: "exited"}, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	assert.Equal(t, []ledger.Kind{ledger.KindRunnerExited}, kinds(exit))

	st = base()
	exit, err = ledger.Exit(st, ledger.RunnerExited{RunnerID: "r1", Reason: "exited"}, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	apply(&st, exit)
	left, err = ledger.Leave(st, "r1")
	require.NoError(t, err)
	assert.Empty(t, left)
}
