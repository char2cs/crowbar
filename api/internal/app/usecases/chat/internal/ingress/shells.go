package ingress

import (
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// toolShellRows are what a finished call of the main agent says of the
// background commands: a command it left running starts a shell, and a meta
// call that stopped a known one ends it. Only a call of those kinds may, so a
// task id any other tool happens to carry starts and ends nothing.
func toolShellRows(st ledger.State, tool *engineagents.ToolEvent, turnID string) ([]ledger.Row, error) {
	if id := tool.SpawnsShell; id != "" && tool.Kind == ledger.ToolKindExecute {
		if _, seen := st.ShellOf(id); !seen {
			row, err := ledger.NewRow(ledger.KindShell, turnID, "", ledger.ShellBody{
				Phase: ledger.ShellStart, ShellID: id, Command: tool.Target, Label: tool.Title,
			})
			return []ledger.Row{row}, err
		}
	}
	if tool.StopsShell != "" && tool.Kind == ledger.ToolKindMeta {
		return endShell(st, tool.StopsShell, ledger.ShellStopped)
	}
	return nil, nil
}

// promptShellRows are the end of the background command a harness-injected
// prompt reports ended; a task that is no live shell of the chat's (a
// subagent's, say) ends nothing.
func promptShellRows(st ledger.State, shell *engineagents.ShellEvent) ([]ledger.Row, error) {
	if shell == nil {
		return nil, nil
	}
	status := shell.Status
	if !ledger.ShellStatusEnds(status) {
		status = ledger.ShellFinished
	}
	return endShell(st, shell.ID, status)
}

func endShell(st ledger.State, id, status string) ([]ledger.Row, error) {
	shell, ok := st.ShellOf(id)
	if !ok || !shell.Live() {
		return nil, nil
	}
	row, err := ledger.NewRow(ledger.KindShell, shell.TurnID, "", ledger.ShellBody{
		Phase: ledger.ShellEnd, ShellID: id, Status: status,
	})
	return []ledger.Row{row}, err
}
