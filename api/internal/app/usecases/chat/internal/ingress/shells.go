package ingress

import (
	"cmp"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// toolShellRows are what a call of the main agent says of the background
// commands: a command it left running starts a shell, a meta call that stopped
// a known one ends it, and the end of a call that is itself a shell ends that
// shell. Only a call of those kinds may, so a task id any other tool happens
// to carry starts and ends nothing.
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

// runOf names where a command ran and its process when its call has one: what
// makes it a shell should input reach it.
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

// callEndStatus is how the end of a call that was a shell ended it.
func callEndStatus(ev engineagents.CanonicalEvent) string {
	switch {
	case ev.Tool.Interrupted:
		return ledger.ShellStopped
	case ev.Kind == engineagents.HookToolFail, ev.Tool.ExitCode != nil && *ev.Tool.ExitCode != 0:
		return ledger.ShellFailed
	}
	return ledger.ShellFinished
}

// mapTerminalInput is input written to the process of a call still running:
// that call outlived the moment it returned, so it is a shell from here on. A
// call this chat holds no open command for starts nothing.
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

// promptShellRows are what a harness-injected prompt reports of a background
// command: its end, or a line it printed. A task that is no live shell of the
// chat's (a subagent's, say) ends and says nothing.
func promptShellRows(st ledger.State, shell *engineagents.ShellEvent) ([]ledger.Row, error) {
	if shell == nil {
		return nil, nil
	}
	if shell.Status == "" {
		return noteShell(st, shell)
	}
	status := shell.Status
	if !ledger.ShellStatusEnds(status) {
		status = ledger.ShellFinished
	}
	return endShell(st, shell.ID, ledger.ShellEnding{Status: status, Summary: shell.Summary})
}

func noteShell(st ledger.State, shell *engineagents.ShellEvent) ([]ledger.Row, error) {
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
