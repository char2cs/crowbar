package ledger_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func shellRow(t *testing.T, body ledger.ShellBody) ledger.Row {
	t.Helper()
	return scoped(t, ledger.KindShell, "T1", "", body)
}

func TestCheck_RefusesAMalformedShellRow(t *testing.T) {
	for name, body := range map[string]ledger.ShellBody{
		"no id":         {Phase: ledger.ShellStart},
		"unknown phase": {Phase: "pause", ShellID: "s"},
		"end no status": {Phase: ledger.ShellEnd, ShellID: "s"},
		"odd status":    {Phase: ledger.ShellEnd, ShellID: "s", Status: "exploded"},
		"odd kind":      {Phase: ledger.ShellStart, ShellID: "s", Kind: "daemon"},
		"empty note":    {Phase: ledger.ShellNote, ShellID: "s"},
	} {
		require.ErrorIs(t, ledger.Check(ledger.State{}, shellRow(t, body)), ledger.ErrMalformedRow, name)
	}
	assert.NoError(t, ledger.Check(ledger.State{}, shellRow(t, ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "s"})))
	assert.NoError(t, ledger.Check(ledger.State{}, shellRow(t, ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellFailed})))
	assert.NoError(t, ledger.Check(ledger.State{}, shellRow(t, ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "s", Kind: ledger.ShellKindMonitor})))
	assert.NoError(t, ledger.Check(ledger.State{}, shellRow(t, ledger.ShellBody{Phase: ledger.ShellNote, ShellID: "s", Tail: "line"})))
}

func TestFold_AStoppingShellStaysLiveUntilItEnds(t *testing.T) {
	start := ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "s"}
	stopping := ledger.ShellBody{Phase: ledger.ShellStopping, ShellID: "s"}
	end := ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellStopped}
	require.NoError(t, ledger.Check(ledger.State{}, shellRow(t, stopping)))

	st := ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), shellRow(t, start), shellRow(t, stopping)))
	shell, _ := st.ShellOf("s")
	assert.True(t, shell.Live())
	assert.True(t, shell.Stopping)

	st = ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), shellRow(t, start), shellRow(t, stopping), shellRow(t, end)))
	shell, _ = st.ShellOf("s")
	assert.False(t, shell.Live())
}

func TestFold_AShellRunsUntilItsFirstEndAndKeepsNoChatBusy(t *testing.T) {
	start := ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "s", Command: "make dev", Label: "dev"}
	st := ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), shellRow(t, start), ended(t, "T1")))

	shell, ok := st.ShellOf("s")
	require.True(t, ok)
	assert.True(t, shell.Live())
	assert.False(t, st.Busy(), "a shell is not the chat's work")

	st = ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), shellRow(t, start), ended(t, "T1"),
		shellRow(t, ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellFailed}),
		shellRow(t, ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellStopped})))
	shell, _ = st.ShellOf("s")
	assert.False(t, shell.Live())
	assert.Equal(t, ledger.ShellFailed, shell.Status, "its first end is its end")
}

func TestShellTail_KeepsTheEndOfTheOutputWithinItsBoundAtALineStart(t *testing.T) {
	assert.Equal(t, "a\nb", ledger.ShellTail("a\nb\n\n"))
	assert.Empty(t, ledger.ShellTail(""))

	long := strings.Repeat("0123456789\n", 400) + "last"
	tail := ledger.ShellTail(long)
	assert.LessOrEqual(t, len(tail), ledger.ShellTailBytes)
	assert.True(t, strings.HasSuffix(tail, "\nlast"))
	assert.True(t, strings.HasPrefix(tail, "0123456789"), "cut at a line start")
}

func TestFold_ARunnersExitStopsItsLiveShellsAndNoOthers(t *testing.T) {
	start := func(turn, id string) ledger.Row {
		return row(t, ledger.KindShell, turn, ledger.ShellBody{Phase: ledger.ShellStart, ShellID: id})
	}
	st := ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), start("T1", "a"), ended(t, "T1"),
		row(t, ledger.KindTurnStarted, "T2", ledger.TurnStarted{RunnerID: "run-2"}), start("T2", "kept"),
		exited(t, "run-1", "crashed", 1)))

	a, _ := st.ShellOf("a")
	assert.Equal(t, ledger.ShellStopped, a.Status, "its process died with the runner")
	kept, _ := st.ShellOf("kept")
	assert.True(t, kept.Live(), "another runner's shell stays")
}
