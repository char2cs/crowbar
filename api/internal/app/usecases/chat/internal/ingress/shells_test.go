package ingress

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func backgroundCall(id, shell string) engineagents.CanonicalEvent {
	ev := toolEvent(engineagents.HookToolPost, id, "run")
	ev.Tool.Kind, ev.Tool.Target, ev.Tool.Title, ev.Tool.SpawnsShell = "execute", "npm run dev", "Dev server", shell
	return ev
}

func shellEnded(id, status string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{
		Kind: engineagents.HookUserPrompt, Shell: &engineagents.ShellEvent{ID: id, Status: status},
	}
}

func shellRowsOf(t *testing.T, rows []ledger.Row) []ledger.ShellBody {
	t.Helper()
	var out []ledger.ShellBody
	for _, r := range rows {
		if r.Kind == ledger.KindShell {
			out = append(out, bodyOf[ledger.ShellBody](t, r))
		}
	}
	return out
}

// A command that went to the background starts a shell; a CLI's retry starts none twice.
func TestMap_ABackgroundedCommandStartsAShellOnce(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("serve it")

	rows, _ := c.mapped(backgroundCall("call-1", "sh-1"), hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindToolCall, ledger.KindShell}, kinds(rows))
	assert.Equal(t, turnID, rows[1].TurnID)
	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellStart, ShellID: "sh-1", Kind: ledger.ShellKindShell, Command: "npm run dev", Label: "Dev server"}},
		shellRowsOf(t, rows))
	again, _ := c.mapped(backgroundCall("call-1", "sh-1"), hooksSource())
	assert.Empty(t, again)
	shell, ok := c.st.ShellOf("sh-1")
	require.True(t, ok)
	assert.True(t, shell.Live())
}

func TestMap_ACallThatIsNoCommandStartsNoShell(t *testing.T) {
	c := liveChatRows(t)
	c.typed("plan")
	ev := backgroundCall("call-1", "7")
	ev.Tool.Kind = "plan"

	rows, _ := c.mapped(ev, hooksSource())

	assert.Equal(t, []ledger.Kind{ledger.KindToolCall}, kinds(rows))
}

func TestMap_ASubagentsBackgroundCommandIsNotTheMainAgents(t *testing.T) {
	c := liveChatRows(t)
	c.typed("delegate")
	ev := backgroundCall("call-1", "sh-1")
	ev.Subagent = &engineagents.SubagentEvent{ID: "A"}

	rows, _ := c.mapped(ev, hooksSource())

	assert.Empty(t, shellRowsOf(t, rows))
}

// The agent stopping a shell, and the provider reporting one ended, each close
// it once; a task that is no shell of the chat's is a subagent's and closes nothing.
func TestMap_AShellEndsByItsStopOrItsNotificationAndByNothingElse(t *testing.T) {
	c := liveChatRows(t)
	c.typed("serve it")
	c.mapped(backgroundCall("call-1", "sh-1"), hooksSource())
	c.mapped(backgroundCall("call-2", "sh-2"), hooksSource())

	stop := toolEvent(engineagents.HookToolPost, "call-3", "stop")
	stop.Tool.Kind, stop.Tool.StopsShell = "meta", "sh-1"
	rows, _ := c.mapped(stop, hooksSource())
	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellEnd, ShellID: "sh-1", Status: ledger.ShellStopped}},
		shellRowsOf(t, rows))

	rows, _ = c.mapped(shellEnded("sh-2", "failed"), hooksSource())
	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellEnd, ShellID: "sh-2", Status: ledger.ShellFailed}},
		shellRowsOf(t, rows))

	for _, ev := range []engineagents.CanonicalEvent{shellEnded("sh-1", "finished"), shellEnded("sh-2", "finished"), shellEnded("agent-9", "finished")} {
		rows, _ = c.mapped(ev, hooksSource())
		assert.Empty(t, shellRowsOf(t, rows), "an ended or unknown task ends nothing")
	}
}

func TestMap_AnUnknownEndStatusReadsFinished(t *testing.T) {
	c := liveChatRows(t)
	c.typed("serve it")
	c.mapped(backgroundCall("call-1", "sh-1"), hooksSource())

	rows, _ := c.mapped(shellEnded("sh-1", "gone"), hooksSource())

	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellEnd, ShellID: "sh-1", Status: ledger.ShellFinished}},
		shellRowsOf(t, rows))
}

// A monitor is a shell of its own kind, and says how long it was set to watch.
func TestMap_AMonitorIsAShellOfItsOwnKind(t *testing.T) {
	c := liveChatRows(t)
	c.typed("watch the log")
	ev := backgroundCall("call-1", "mon-1")
	ev.Tool.Target, ev.Tool.Title, ev.Tool.Cwd = "tail -f app.log", "Watch the log", "/work"
	ev.Tool.ShellKind, ev.Tool.ShellPersistent, ev.Tool.ShellTimeoutMS = "monitor", true, 300000

	rows, _ := c.mapped(ev, hooksSource())

	assert.Equal(t, []ledger.ShellBody{{
		Phase: ledger.ShellStart, ShellID: "mon-1", Kind: ledger.ShellKindMonitor, Command: "tail -f app.log",
		Label: "Watch the log", Cwd: "/work", Persistent: true, TimeoutMS: 300000,
	}}, shellRowsOf(t, rows))
}

func TestMap_ANotificationWithoutAnEndIsALineOfTheShellNotItsEnd(t *testing.T) {
	c := liveChatRows(t)
	c.typed("watch")
	c.mapped(backgroundCall("call-1", "mon-1"), hooksSource())

	line := engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Shell: &engineagents.ShellEvent{ID: "mon-1", Note: "ERROR db down"}}
	rows, _ := c.mapped(line, hooksSource())
	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellNote, ShellID: "mon-1", Tail: "ERROR db down"}}, shellRowsOf(t, rows))
	c.commit(rows...)
	shell, _ := c.st.ShellOf("mon-1")
	assert.True(t, shell.Live())

	other := engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Shell: &engineagents.ShellEvent{ID: "agent-9", Note: "x"}}
	rows, _ = c.mapped(other, hooksSource())
	assert.Empty(t, shellRowsOf(t, rows), "a line of no shell of the chat's says nothing")
}

func TestMap_ANotificationsSummaryRidesItsEnd(t *testing.T) {
	c := liveChatRows(t)
	c.typed("serve it")
	c.mapped(backgroundCall("call-1", "sh-1"), hooksSource())

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind:  engineagents.HookUserPrompt,
		Shell: &engineagents.ShellEvent{ID: "sh-1", Status: "finished", Summary: "Background command finished"},
	}, hooksSource())

	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellEnd, ShellID: "sh-1", Status: ledger.ShellFinished, Summary: "Background command finished"}},
		shellRowsOf(t, rows))
}

func commandPre() engineagents.CanonicalEvent {
	ev := toolEvent(engineagents.HookToolPre, "item-1", "commandExecution")
	ev.Tool.Kind, ev.Tool.Target, ev.Tool.Cwd, ev.Tool.ShellRef = "execute", "npm run dev", "/work", "10527"
	return ev
}

func terminalInput(id, ref string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{Kind: engineagents.HookTerminalInput, Tool: &engineagents.ToolEvent{ID: id, ShellRef: ref}}
}

// A command that returned is no shell until input reaches its process: only then
// is it known to have outlived the call. It ends with the call, and says how.
func TestMap_ACommandBecomesAShellOnlyOnceItsProcessGetsInputAndEndsWithItsCall(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("serve it")
	rows, _ := c.mapped(commandPre(), hooksSource())
	c.commit(rows...)
	assert.Empty(t, shellRowsOf(t, rows), "a started command is no shell")

	rows, _ = c.mapped(terminalInput("item-1", "10527"), hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindShell}, kinds(rows))
	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, []ledger.ShellBody{{
		Phase: ledger.ShellStart, ShellID: "item-1", Kind: ledger.ShellKindShell, Command: "npm run dev", Cwd: "/work", Ref: "10527",
	}}, shellRowsOf(t, rows))
	c.commit(rows...)
	rows, _ = c.mapped(terminalInput("item-1", "10527"), hooksSource())
	assert.Empty(t, rows, "a second poll starts nothing")

	post := toolEvent(engineagents.HookToolPost, "item-1", "commandExecution")
	post.Tool.Kind, post.Tool.Output = "execute", strings.Repeat("x", 3000)+"\nlast line\n"
	code := 2
	post.Tool.ExitCode = &code
	rows, _ = c.mapped(post, hooksSource())
	ends := shellRowsOf(t, rows)
	require.Len(t, ends, 1)
	assert.Equal(t, ledger.ShellBody{
		Phase: ledger.ShellEnd, ShellID: "item-1", Status: ledger.ShellFailed, ExitCode: &code, Tail: ends[0].Tail,
	}, ends[0])
	assert.LessOrEqual(t, len(ends[0].Tail), ledger.ShellTailBytes)
	assert.True(t, strings.HasSuffix(ends[0].Tail, "last line"))
}

func TestMap_InputToACommandThatIsNotOpenStartsNoShell(t *testing.T) {
	c := liveChatRows(t)
	c.typed("run")

	rows, _ := c.mapped(terminalInput("nothing", "9"), hooksSource())
	assert.Empty(t, rows, "no such command")

	pre := commandPre()
	rows, _ = c.mapped(pre, hooksSource())
	c.commit(rows...)
	post := toolEvent(engineagents.HookToolPost, "item-1", "commandExecution")
	post.Tool.Kind = "execute"
	rows, _ = c.mapped(post, hooksSource())
	c.commit(rows...)
	rows, _ = c.mapped(terminalInput("item-1", "10527"), hooksSource())
	assert.Empty(t, rows, "a command that ended is no shell")
}

// Input names the process, not only the call: the open command holding it is the one.
func TestMap_InputThatNamesOnlyTheProcessFindsItsOpenCommand(t *testing.T) {
	c := liveChatRows(t)
	c.typed("serve it")
	rows, _ := c.mapped(commandPre(), hooksSource())
	c.commit(rows...)

	rows, _ = c.mapped(terminalInput("write-stdin-call", "10527"), hooksSource())

	assert.Equal(t, "item-1", shellRowsOf(t, rows)[0].ShellID)
}

func TestMap_ACallInterruptedEndsItsShellStopped(t *testing.T) {
	c := liveChatRows(t)
	c.typed("serve it")
	rows, _ := c.mapped(commandPre(), hooksSource())
	c.commit(rows...)
	rows, _ = c.mapped(terminalInput("item-1", "10527"), hooksSource())
	c.commit(rows...)

	fail := toolEvent(engineagents.HookToolFail, "item-1", "commandExecution")
	fail.Tool.Kind, fail.Tool.Interrupted = "execute", true
	rows, _ = c.mapped(fail, hooksSource())

	assert.Equal(t, ledger.ShellStopped, shellRowsOf(t, rows)[0].Status)
}
