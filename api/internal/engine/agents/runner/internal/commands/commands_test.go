package commands_test

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/runner/internal/commands"
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
	require.Equal(t, time.Unix(1, 0), got.StartedAt)
}

// TestRegression_StartAcceptsARunnerWhoseProcessIsNotAPTY is the orphan-PTY
// fix's own invariant change (usecases/chat/internal/runner/apirunner.go): a
// runner driven by an api connection forks no terminal at all, so it has no
// terminal session to name — and naming a fake one would put an id in the
// projection that the terminal engine has never heard of, which the DTO, the
// terminal-wait sweep and boot reconciliation all read.
//
// A runner still has exactly ONE process; what changed is that the process is
// not always a PTY. Chat placement (I1) is untouched — it is the invariant
// this aggregate actually exists to hold.
func TestRegression_StartAcceptsARunnerWhoseProcessIsNotAPTY(t *testing.T) {
	c := commands.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "", ChatID: "c1", Now: time.Unix(1, 0),
	}
	require.NoError(t, c.Validate(nil))
	require.Empty(t, c.EmitEvent(nil).TerminalSession,
		"the absence has to survive into the projection; every reader already treats it as no terminal")
}

func TestStart_RejectsMissingChat(t *testing.T) {
	c := commands.Start{RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude", TerminalSession: "pty1"}
	require.Error(t, c.Validate(nil), "a runner always points at exactly one chat (spec I1)")
}

func TestReport_KeepsWhatTheCLIDidNotReport(t *testing.T) {
	cur := &agents.Runner{ID: "r1", CurrentChatID: "c1", LaunchModel: "opus", ReportedModel: "m1", ReportedEffort: "high"}
	require.ErrorIs(t, commands.Report{RunnerID: "r1"}.Validate(cur), asynxModels.ErrValidation, "nothing reported")
	require.ErrorIs(t, commands.Report{RunnerID: "r1", Model: "m2"}.Validate(nil), asynxModels.ErrValidation)

	got := commands.Report{RunnerID: "r1", Model: "m2"}.EmitEvent(cur)
	require.Equal(t, "m2", got.ReportedModel)
	require.Equal(t, "high", got.ReportedEffort, "an effort the CLI did not report is left as it was")
	require.Equal(t, "opus", got.LaunchModel, "what Crowbar asked for is never overwritten")
	require.Equal(t, "c1", got.CurrentChatID)
}

// The runner has NO status field to consult (spec §2). Exit is a tombstone for
// audit; liveness is answered by the chat_liveness projection, which drops the
// row, and ultimately by the PTY.
func TestExit_TombstonesForAudit(t *testing.T) {
	cur := &agents.Runner{ID: "r1", CurrentChatID: "c1"}
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
	cur := &agents.Runner{ID: "r1", CurrentChatID: "c1", TerminalSession: "pty1"}
	c := commands.Displace{RunnerID: "r1"}
	require.NoError(t, c.Validate(cur))

	got := c.EmitEvent(cur)
	require.Empty(t, got.CurrentChatID, "the runner is on no chat")
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
	require.Equal(t, "runner.started.r1", commands.Start{RunnerID: "r1"}.EventName())
	require.Equal(t, "runner.session_bound.r1", commands.Report{RunnerID: "r1"}.EventName())
	require.Equal(t, "runner.displaced.r1", commands.Displace{RunnerID: "r1"}.EventName())
	require.Equal(t, "runner.exited.r1", commands.Exit{RunnerID: "r1"}.EventName())
}
