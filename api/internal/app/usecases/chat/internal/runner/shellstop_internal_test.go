package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func runnersWithShell(t *testing.T, bodies ...ledger.ShellBody) *Runners {
	t.Helper()
	rs := &Runners{ledger: newTestLedger(t)}
	_, _, err := rs.ledger.Do(t.Context(), "chat-1", func(ledger.State) ([]ledger.Row, error) {
		rows := []ledger.Row{}
		turn, err := ledger.NewRow(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{RunnerID: "run-1"})
		if err != nil {
			return nil, err
		}
		rows = append(rows, turn)
		for _, b := range bodies {
			row, err := ledger.NewRow(ledger.KindShell, "T1", "", b)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
		return rows, nil
	})
	require.NoError(t, err)
	return rs
}

func TestStopShell_OnlyARunningShellOfTheChatCanBeStopped(t *testing.T) {
	rs := runnersWithShell(t,
		ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "done"},
		ledger.ShellBody{Phase: ledger.ShellEnd, ShellID: "done", Status: ledger.ShellFinished},
	)

	for _, id := range []string{"done", "never-started"} {
		err := rs.StopShell(context.Background(), "chat-1", id)

		require.ErrorIs(t, err, problem.ErrConflict, id)
	}
}

func TestStopShell_AskingAgainWhileItIsStoppingDoesNothing(t *testing.T) {
	rs := runnersWithShell(t,
		ledger.ShellBody{Phase: ledger.ShellStart, ShellID: "sh"},
		ledger.ShellBody{Phase: ledger.ShellStopping, ShellID: "sh"},
	)

	require.NoError(t, rs.StopShell(context.Background(), "chat-1", "sh"))
}
