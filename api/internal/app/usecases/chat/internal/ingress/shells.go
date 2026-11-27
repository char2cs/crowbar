package ingress

import (
	"cmp"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func toolShellRows(st ledger.State, ev engineagents.CanonicalEvent, scope, turnID string) ([]ledger.Row, error) {
	tool := ev.Tool
	if scope != "" {
		return nil, nil
	}
	if id := tool.SpawnsShell; id != "" && tool.Kind == ledger.ToolKindExecute {
		if _, seen := st.ShellOf(id); !seen {
			row, err := ledger.NewRow(ledger.KindShell, turnID, "", ledger.ShellBody{
				Phase: ledger.ShellStart, ShellID: id, Kind: shellKind(tool.ShellKind),
				Command: tool.Target, Label: tool.Title, Cwd: tool.Cwd, Ref: tool.ShellRef,
				Persistent: tool.ShellPersistent, TimeoutMS: tool.ShellTimeoutMS,
			})
			return []ledger.Row{row}, err
		}
	}
	if tool.StopsShell != "" && tool.Kind == ledger.ToolKindMeta {
		return endShell(st, tool.StopsShell, ledger.ShellEnding{Status: ledger.ShellStopped})
	}
	return endShell(st, tool.ID, ledger.ShellEnding{
		Status: callEndStatus(ev), ExitCode: tool.ExitCode, Tail: ledger.ShellTail(tool.Output),
	})
}

func runOf(body *ledger.ToolCall, tool *engineagents.ToolEvent) {
	if tool.ShellRef != "" {
		body.Cwd, body.Ref = tool.Cwd, tool.ShellRef
	}
}

func shellKind(kind string) string {
	if kind == ledger.ShellKindMonitor {
		return kind
	}
	return ledger.ShellKindShell
}

func callEndStatus(ev engineagents.CanonicalEvent) string {
	switch {
	case ev.Tool.Interrupted:
		return ledger.ShellStopped
	case ev.Kind == engineagents.HookToolFail, ev.Tool.ExitCode != nil && *ev.Tool.ExitCode != 0:
		return ledger.ShellFailed
	}
	return ledger.ShellFinished
}

func mapTerminalInput(st ledger.State, ev engineagents.CanonicalEvent, _ Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
	tool := ev.Tool
	if tool == nil || ev.Subagent != nil {
		return nil, Effects{}, nil
	}
	id := tool.ID
	if !st.ToolOpen("", id) {
		var found bool
		if id, found = st.OpenToolOfRef("", tool.ShellRef); !found {
			return nil, Effects{}, nil
		}
	}
	kind, command := st.ToolAim("", id)
	if _, seen := st.ShellOf(id); seen || kind != ledger.ToolKindExecute {
		return nil, Effects{}, nil
	}
	turnID, _ := st.ToolTurn("", id)
	cwd, ref := st.ToolRun("", id)
	row, err := ledger.NewRow(ledger.KindShell, turnID, "", ledger.ShellBody{
		Phase: ledger.ShellStart, ShellID: id, Kind: ledger.ShellKindShell,
		Command: command, Cwd: cwd, Ref: cmp.Or(ref, tool.ShellRef),
	})
	return []ledger.Row{row}, Effects{}, err
}

func promptShellRows(st ledger.State, shells []engineagents.ShellEvent) ([]ledger.Row, error) {
	var rows []ledger.Row
	for _, shell := range shells {
		got, err := promptShellRow(st, shell)
		if err != nil {
			return nil, err
		}
		rows = append(rows, got...)
	}
	return rows, nil
}

func promptShellRow(st ledger.State, shell engineagents.ShellEvent) ([]ledger.Row, error) {
	status := cmp.Or(shell.Status, statusOfExit(shell.ExitCode))
	if status == "" {
		return noteShell(st, shell)
	}
	if !ledger.ShellStatusEnds(status) {
		status = ledger.ShellFinished
	}
	return endShell(st, shell.ID, ledger.ShellEnding{Status: status, ExitCode: shell.ExitCode, Summary: shell.Summary})
}

func statusOfExit(code *int) string {
	switch {
	case code == nil:
		return ""
	case *code == 0:
		return ledger.ShellFinished
	}
	return ledger.ShellFailed
}

func noteShell(st ledger.State, shell engineagents.ShellEvent) ([]ledger.Row, error) {
	known, ok := st.ShellOf(shell.ID)
	line := ledger.ShellTail(cmp.Or(shell.Note, shell.Summary))
	if !ok || !known.Live() || line == "" {
		return nil, nil
	}
	row, err := ledger.NewRow(ledger.KindShell, known.TurnID, "", ledger.ShellBody{
		Phase: ledger.ShellNote, ShellID: shell.ID, Tail: line,
	})
	return []ledger.Row{row}, err
}

func endShell(st ledger.State, id string, end ledger.ShellEnding) ([]ledger.Row, error) {
	shell, ok := st.ShellOf(id)
	if !ok || !shell.Live() {
		return nil, nil
	}
	row, err := ledger.NewRow(ledger.KindShell, shell.TurnID, "", ledger.ShellBody{
		Phase: ledger.ShellEnd, ShellID: id, Status: end.Status, ExitCode: end.ExitCode, Tail: end.Tail, Summary: end.Summary,
	})
	return []ledger.Row{row}, err
}
