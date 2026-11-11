package ledger_test

import (
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
	} {
		assert.ErrorIs(t, ledger.Check(ledger.State{}, shellRow(t, body)), ledger.ErrMalformedRow, name)
	}
	assert.NoError(t, ledger.Check(ledger.State{}, shellRow(t, ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "s"})))
	assert.NoError(t, ledger.Check(ledger.State{}, shellRow(t, ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellFailed})))
}

func TestFold_AShellRunsUntilItsFirstEndAndKeepsNoChatBusy(t *testing.T) {
	start := ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "s", Command: "make dev", Label: "dev"}
	st := ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), shellRow(t, start), ended(t, "T1")))

	shell, ok := st.ShellOf("s")
	require.True(t, ok)
	assert.Equal(t, "make dev", shell.Command)
	assert.True(t, shell.Live())
	assert.False(t, st.Busy(), "a shell is not the chat's work")

	st = ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), shellRow(t, start), ended(t, "T1"),
		shellRow(t, ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellFailed}),
		shellRow(t, ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "s", Status: ledger.ShellStopped})))
	shell, _ = st.ShellOf("s")
	assert.False(t, shell.Live())
	assert.Equal(t, ledger.ShellFailed, shell.Status, "its first end is its end")
}
