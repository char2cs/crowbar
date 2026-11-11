package ingress

import (
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
	assert.Equal(t, []ledger.ShellBody{{Phase: ledger.ShellStart, ShellID: "sh-1", Command: "npm run dev", Label: "Dev server"}},
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
