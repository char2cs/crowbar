package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func started(t *testing.T, runnerID string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindRunnerStarted, "", ledger.RunnerStarted{RunnerID: runnerID, Provider: "claude"})
}

func exited(t *testing.T, runnerID, reason string, code int) ledger.Row {
	t.Helper()
	return row(t, ledger.KindRunnerExited, "", ledger.RunnerExited{RunnerID: runnerID, Reason: reason, ExitCode: code})
}

func TestFold_LiveRunnerAndLastExit(t *testing.T) {
	rows := sequenced(t, started(t, "r1"), exited(t, "r1", "stopped", 0), started(t, "r2"))
	st := ledger.Fold("c1", rows)

	live, ok := st.LiveRunner()
	require.True(t, ok)
	assert.Equal(t, "r2", live.RunnerID)
	assert.Equal(t, "claude", live.Provider)
	last, ok := st.LastExit()
	require.True(t, ok)
	assert.Equal(t, ledger.RunnerExit{RunnerID: "r1", Reason: "stopped", At: rows[1].At}, last)
	assert.True(t, st.HasOpenWork(), "a live runner is unfinished work")

	end := exited(t, "r2", "exited", 143)
	end.Seq = 4
	st.Apply(end)
	_, ok = st.LiveRunner()
	assert.False(t, ok)
	last, ok = st.LastExit()
	require.True(t, ok)
	assert.Equal(t, "exited", last.Reason)
	assert.Equal(t, 143, last.ExitCode)
	assert.False(t, st.HasOpenWork())
}

// An exit naming another runner is late: it leaves the live one live.
func TestFold_AnExitOfAnotherRunnerKeepsTheLiveOne(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t, started(t, "r2"), exited(t, "r1", "displaced", 0)))

	live, ok := st.LiveRunner()
	require.True(t, ok)
	assert.Equal(t, "r2", live.RunnerID)
	last, ok := st.LastExit()
	require.True(t, ok)
	assert.Equal(t, "r1", last.RunnerID)
}

func TestFold_NoRunnerHasNoExit(t *testing.T) {
	st := ledger.Fold("c1", nil)
	_, ok := st.LiveRunner()
	assert.False(t, ok)
	_, ok = st.LastExit()
	assert.False(t, ok)
}

func TestCheck_RunnerRowsNameTheirRunner(t *testing.T) {
	for name, r := range map[string]ledger.Row{
		"started without a runner":   row(t, ledger.KindRunnerStarted, "", ledger.RunnerStarted{Provider: "claude"}),
		"started without a provider": row(t, ledger.KindRunnerStarted, "", ledger.RunnerStarted{RunnerID: "r1"}),
		"exited without a runner":    row(t, ledger.KindRunnerExited, "", ledger.RunnerExited{Reason: "exited"}),
		"exited without a reason":    row(t, ledger.KindRunnerExited, "", ledger.RunnerExited{RunnerID: "r1"}),
	} {
		require.ErrorIs(t, ledger.Check(ledger.State{}, r), ledger.ErrMalformedRow, name)
	}
	require.NoError(t, ledger.Check(ledger.State{}, started(t, "r1")))
	require.NoError(t, ledger.Check(ledger.State{}, exited(t, "r1", "exited", 0)))
}

func TestExit_EndsTheRunnerAndSettlesOnlyWhatItCarried(t *testing.T) {
	rows, err := ledger.Exit(carriedPrompts(t), ledger.RunnerExited{RunnerID: "run-1", Reason: "stopped"}, ledger.ReasonDaemonRestart)

	require.NoError(t, err)
	require.NotEmpty(t, rows)
	assert.Equal(t, ledger.KindRunnerExited, rows[0].Kind)
	assert.Equal(t, []ledger.PromptSettled{{RequestID: "r1", Reason: ledger.ReasonDaemonRestart}}, settled(t, rows[1:]),
		"only the pending prompt run-1 carried; r3 was delivered")
}
