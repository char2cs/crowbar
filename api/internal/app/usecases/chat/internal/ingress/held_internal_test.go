package ingress

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// apiChild is a chat on session main whose agent has started spawning a
// subagent over its api connection, the call not yet bound.
func apiChild(t *testing.T) *recorderFixture {
	t.Helper()
	f := newRecorderFixture(t, "codex")
	bound, err := ledger.Announce(f.state(t), "codex", "main", false)
	require.NoError(t, err)
	_, _, err = f.ledger.Append(t.Context(), testChat, bound...)
	require.NoError(t, err)
	f.api(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, SessionID: "main", Message: "delegate", TurnID: "pt-1"})
	f.api(t, spawnOn("main", engineagents.HookToolPre, ""))
	return f
}

func (f *recorderFixture) api(t *testing.T, ev engineagents.CanonicalEvent) {
	t.Helper()
	_, _, err := f.r.Dispatch(t.Context(), f.runner, f.agent, ev, []byte(`{}`), engineagents.ChannelAPI)
	require.NoError(t, err)
}

func spawnOn(session, kind, scope string) engineagents.CanonicalEvent {
	ev := spawnCall(kind, scope)
	ev.SessionID = session
	return ev
}

func (f *recorderFixture) unclaimedCount(t *testing.T) int {
	t.Helper()
	n, err := f.ledger.DeadLetterCount(t.Context(), reasonUnclaimedScope)
	require.NoError(t, err)
	return n
}

func TestIngress_ChildFramesBeforeTheBindAreHeldThenDrained(t *testing.T) {
	f := apiChild(t)

	f.api(t, engineagents.CanonicalEvent{Kind: engineagents.HookSessionStart, SessionID: "child"})
	f.api(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre, SessionID: "child", Tool: &engineagents.ToolEvent{ID: "c1", Name: "Bash"},
	})
	assert.Empty(t, f.runners.started, "a child's thread start never rebinds the chat")
	conv, ok := f.state(t).Conversation("codex")
	require.True(t, ok)
	assert.Equal(t, "main", conv.SessionID)

	f.api(t, spawnOn("main", engineagents.HookToolPost, "child"))

	rows := f.rows(t)
	last := rows[len(rows)-1]
	assert.Equal(t, ledger.KindToolCall, last.Kind)
	assert.Equal(t, "child", last.Scope, "the held frame is drained into its scope on the bind")
	assert.Zero(t, f.unclaimedCount(t))
	assert.True(t, f.state(t).ScopeWaits("child"))

	// C20: the parent's end restates nothing; the child keeps the chat working until its own end.
	f.api(t, engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "main", TurnID: "pt-1"})
	assert.True(t, f.state(t).Busy(), "a backgrounded subagent keeps the chat working")
	f.api(t, engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "child", Message: "done"})
	assert.False(t, f.state(t).Busy())
}

func TestIngress_HeldFramesTheParentTurnsEndLeavesAreDeadLettered(t *testing.T) {
	f := apiChild(t)
	f.api(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre, SessionID: "child", Tool: &engineagents.ToolEvent{ID: "c1", Name: "Bash"},
	})

	f.api(t, engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "main", TurnID: "pt-1"})

	assert.Equal(t, 1, f.unclaimedCount(t))
	assert.False(t, f.state(t).Busy(), "the spawn nothing bound is abandoned with its turn")
	_, _, err := f.r.Dispatch(t.Context(), f.runner, f.agent, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre, SessionID: "child", Tool: &engineagents.ToolEvent{ID: "c2", Name: "Bash"},
	}, []byte(`{}`), engineagents.ChannelAPI)
	require.ErrorIs(t, err, engineagents.ErrForeignConversation, "with nothing starting, another session is foreign")
}

func TestIngress_AFullChatDeadLettersItsOldestHeldFrame(t *testing.T) {
	f := apiChild(t)
	for i := range heldPerChat + 1 {
		f.api(t, engineagents.CanonicalEvent{
			Kind: engineagents.HookToolPre, SessionID: "child", Tool: &engineagents.ToolEvent{ID: fmt.Sprintf("c%d", i), Name: "Bash"},
		})
	}

	assert.Equal(t, 1, f.unclaimedCount(t))
	f.api(t, spawnOn("main", engineagents.HookToolPost, "child"))
	var drained []string
	for _, r := range f.rows(t) {
		if r.Kind == ledger.KindToolCall && r.Scope == "child" {
			drained = append(drained, bodyOf[ledger.ToolCall](t, r).ToolID)
		}
	}
	require.Len(t, drained, heldPerChat)
	assert.Equal(t, "c1", drained[0], "the oldest went first, the rest in arrival order")
}

func TestIngress_AnAskOfAnUnboundConversationIsNeverHeld(t *testing.T) {
	f := apiChild(t)

	_, _, err := f.r.Dispatch(t.Context(), f.runner, f.agent, engineagents.CanonicalEvent{
		Kind: engineagents.HookPermission, SessionID: "child",
		Choice: &engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"},
	}, []byte(`{}`), engineagents.ChannelAPI)

	require.ErrorIs(t, err, engineagents.ErrForeignConversation, "its sender waits on an answer a hold could never give")
}
