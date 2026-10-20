package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// runnersForDisplace builds the smallest Runners that can run displaceForSwitch
// on chat-1, with runner r-1 live on it when live is set.
func runnersForDisplace(t *testing.T, live bool) *Runners {
	t.Helper()
	liveTable := newLiveTable()
	if live {
		liveTable = liveWith(t, engineagents.Runner{ID: "r-1", CurrentChatID: "chat-1"})
	}
	return &Runners{
		ws:       fakeWSReader{chatsDir: t.TempDir()},
		ledger:   newTestLedger(t),
		live:     liveTable,
		chatLock: newTestLock(),
		parks:    newParks(),
	}
}

// A switch never waits out a running turn, and never forces it: it is refused.
func TestDisplaceForSwitch_ARunningTurnIsRefusedNotWaitedFor(t *testing.T) {
	rs := runnersForDisplace(t, true /* live */)
	started, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "r-1", Provider: "claude"})
	require.NoError(t, err)
	turnRow, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{RunnerID: "r-1"})
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", started, turnRow)
	require.NoError(t, err)

	h := rs.acquire(t.Context(), "chat-1")
	defer h.release()
	retry, err := rs.displaceForSwitch(t.Context(), h, domain.Chat{ID: "chat-1"})

	require.ErrorIs(t, err, ErrPromptBusy)
	assert.False(t, retry)
	_, live := rs.live.byChat("chat-1")
	assert.True(t, live, "the running CLI is not quit")
}

// A prompt steered into the turn the switch waited out is waited for too: the
// switch goes round again to its pending-prompt wait rather than refusing.
func TestDisplaceForSwitch_APromptSteeredAfterTheWaitsSendsTheSwitchRoundAgain(t *testing.T) {
	rs := runnersForDisplace(t, true /* live */)
	appendPrompt(t, rs.ledger, "steered", "r-1")

	h := rs.acquire(t.Context(), "chat-1")
	defer h.release()
	retry, err := rs.displaceForSwitch(t.Context(), h, domain.Chat{ID: "chat-1"})

	require.NoError(t, err)
	assert.True(t, retry)
	assert.Equal(t, ledger.PromptStatusPending, promptReceipt(t, rs.ledger, "steered").Status,
		"its live carrier still owes it")
}

// TestRegression_QuitOutgoingCLI_TerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY:
// a provider switch on a chat attached to its native view must end that view
// too, not only the companion PTY runner.TerminalSession still names.
func TestRegression_QuitOutgoingCLI_TerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY(t *testing.T) {
	term := &fakeTermForAttach{}
	rs := retiring(t, &Runners{
		attached: newAttachRegistry(),
		apiConns: newAPIConnRegistry(),
		term:     term,
		agents:   stubAgentsForInterrupt{},
		turns:    noopTurns{},
	})
	rs.live.update("runner-1", func(r *engineagents.Runner) { r.TerminalSession = "companion-pty-term" })
	rs.attached.set("runner-1", attachedView{termSessID: "native-view-term"})

	require.NoError(t, rs.quitOutgoingCLI(context.Background(), "chat-1"))

	assert.Contains(t, term.terminated, "companion-pty-term",
		"the original companion PTY is still a real process and must still be torn down")
	assert.Contains(t, term.terminated, "native-view-term",
		"the actually-attached native view PTY — the one the user was looking at — must be torn down too")
	_, stillAttached := rs.attached.get("runner-1")
	assert.False(t, stillAttached, "the attach registry entry must be forgotten, or it answers for a runner nothing will revisit")
	assert.Equal(t, domain.AgentExitDisplaced, retiredAs(t, rs), "the outgoing runner must still be displaced from the chat")
}
