package commands_test

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestStart_EmitsRunnerOnItsChat(t *testing.T) {
	c := commands.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	}
	require.NoError(t, c.Validate(nil))
	got := c.EmitEvent(nil)
	require.Equal(t, "r1", got.ID)
	require.Equal(t, "c1", got.CurrentChatID)
	require.Equal(t, "pty1", got.TerminalSession)
	require.Empty(t, got.CurrentSession, "no conversation is bound until the provider announces one")
	require.Zero(t, got.CurrentSessionSince, "nothing is bound yet, so nothing has a since")
}

func TestStart_RejectsMissingChat(t *testing.T) {
	c := commands.Start{RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude", TerminalSession: "pty1"}
	require.Error(t, c.Validate(nil), "a runner always points at exactly one chat (spec I1)")
}

// Move is THE command the whole refactor exists for: one write, one aggregate.
// It can never fail on the state of the destination chat, because it does not
// touch the destination chat. That is what makes the torn write unrepresentable.
func TestMove_RepointsRunnerAtomically(t *testing.T) {
	cur := &domain.AgentRunner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", CurrentChatID: "c1", CurrentSession: "s1",
	}
	c := commands.Move{RunnerID: "r1", ToChatID: "c2", SessionID: "s2", Now: time.Unix(2, 0)}
	require.NoError(t, c.Validate(cur))
	got := c.EmitEvent(cur)
	require.Equal(t, "c2", got.CurrentChatID)
	require.Equal(t, "s2", got.CurrentSession)
	require.Equal(t, "pty1", got.TerminalSession, "the PTY travels with the runner — the terminal never changes")
	require.Equal(t, "claude", got.ProviderID)
	require.Equal(t, time.Unix(2, 0), got.CurrentSessionSince,
		"the move carries WHEN the conversation was entered — the runner's spawn time is not it")
}

func TestMove_RejectsUnknownRunner(t *testing.T) {
	c := commands.Move{RunnerID: "r1", ToChatID: "c2", SessionID: "s2", Now: time.Unix(2, 0)}
	require.Error(t, c.Validate(nil))
}

// A zero Now is a conversation with no opening time. It would stamp FirstSeenAt
// zero and drop conversation ordering back onto insertion order — silently
// reintroducing the inversion bug with no test failing. Refuse it at the door.
func TestMove_RejectsZeroNow(t *testing.T) {
	cur := &domain.AgentRunner{ID: "r1", CurrentChatID: "c1", CurrentSession: "s1"}
	c := commands.Move{RunnerID: "r1", ToChatID: "c2", SessionID: "s2"}
	require.Error(t, c.Validate(cur), "a move must say WHEN the conversation was entered")
}

func TestBindSession_RejectsZeroNow(t *testing.T) {
	cur := &domain.AgentRunner{ID: "r1", CurrentChatID: "c1"}
	c := commands.BindSession{RunnerID: "r1", SessionID: "s1"}
	require.Error(t, c.Validate(cur), "a bind must say WHEN the conversation opened")
}

func TestBindSession_SetsConversationWithoutMovingChat(t *testing.T) {
	cur := &domain.AgentRunner{ID: "r1", CurrentChatID: "c1"}
	c := commands.BindSession{RunnerID: "r1", SessionID: "s1", Now: time.Unix(1, 0)}
	require.NoError(t, c.Validate(cur))
	got := c.EmitEvent(cur)
	require.Equal(t, "s1", got.CurrentSession)
	require.Equal(t, "c1", got.CurrentChatID)
	require.Equal(t, time.Unix(1, 0), got.CurrentSessionSince,
		"the bind carries WHEN the conversation opened, so history can order by it")
}

// The runner has NO status field to consult (spec §2). Exit is a tombstone for
// audit; liveness is answered by the chat_liveness projection, which drops the
// row, and ultimately by the PTY.
func TestExit_TombstonesForAudit(t *testing.T) {
	cur := &domain.AgentRunner{ID: "r1", CurrentChatID: "c1"}
	c := commands.Exit{RunnerID: "r1", Now: time.Unix(9, 0)}
	require.NoError(t, c.Validate(cur))
	got := c.EmitEvent(cur)
	require.NotNil(t, got.ExitedAt)
	require.Equal(t, time.Unix(9, 0), *got.ExitedAt)
}

// Displace states a PLACEMENT fact — "this runner is no longer on any chat" — and says
// NOTHING about liveness. The row survives; only the pointer is cleared. That distinction
// is the whole reason the command is allowed to exist: Crowbar owns placement outright,
// so it may write it the moment it decides it, while liveness stays the PTY's alone.
func TestDisplace_ClearsPlacementWithoutTouchingLiveness(t *testing.T) {
	cur := &domain.AgentRunner{
		ID: "r1", CurrentChatID: "c1", CurrentSession: "s1",
		CurrentSessionSince: time.Unix(5, 0), TerminalSession: "pty1",
	}
	c := commands.Displace{RunnerID: "r1"}
	require.NoError(t, c.Validate(cur))

	got := c.EmitEvent(cur)
	require.Empty(t, got.CurrentChatID, "the runner is on no chat")
	require.Empty(t, got.CurrentSession, "and holds no conversation")
	require.True(t, got.CurrentSessionSince.IsZero(), "a timestamp must not outlive the fact it stamps")
	require.Nil(t, got.ExitedAt, "displacement asserts NOTHING about whether the process is alive")
	require.Equal(t, "pty1", got.TerminalSession, "it still has its PTY — it is still running")

	// The input is untouched (purity), and the command is idempotent.
	require.Equal(t, "c1", cur.CurrentChatID)
	require.NoError(t, c.Validate(&got), "displacing an already-displaced runner is a no-op, not an error")
}

func TestDisplace_RejectsUnknownRunner(t *testing.T) {
	require.ErrorIs(t, commands.Displace{RunnerID: "nope"}.Validate(nil), asynxModels.ErrValidation)
}

func TestEventNames_AreAgentRunnerScoped(t *testing.T) {
	require.Equal(t, "agentrunner.started.r1", commands.Start{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.session_bound.r1", commands.BindSession{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.moved.r1", commands.Move{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.displaced.r1", commands.Displace{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.exited.r1", commands.Exit{RunnerID: "r1"}.EventName())
}
