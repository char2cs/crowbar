package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubRunnerStoreLiveness answers only LiveRunnerForChat, either with a runner or
// with ErrNotFound — the two answers displaceForSwitch's work check now turns on.
type stubRunnerStoreLiveness struct {
	agentrunner.EventStore
	runner engineagents.Runner
	live   bool
}

func (s stubRunnerStoreLiveness) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	if !s.live {
		return engineagents.Runner{}, agentrunner.ErrNotFound
	}
	return s.runner, nil
}

func (s stubRunnerStoreLiveness) LiveRunnersForChat(
	context.Context, string,
) ([]engineagents.Runner, error) {
	if !s.live {
		return nil, nil
	}
	return []engineagents.Runner{s.runner}, nil
}

// runnersForDisplace builds the smallest Runners that can run displaceForSwitch:
// a real turn-start gate and in-flight registry, an empty ledger (so the
// pending-prompt guard passes), and the two answers under test.
func runnersForDisplace(t *testing.T, working, live bool) *Runners {
	t.Helper()
	home := t.TempDir()
	return &Runners{
		ws:            fakeWSReader{chatsDir: home},
		ledger:        newTestLedger(t),
		turnStarts:    inflight.NewGate(),
		inflightTurns: inflight.NewTurns(),
		turns:         stubTurnsForAttach{working: working},
		runnerStore: stubRunnerStoreLiveness{
			live:   live,
			runner: engineagents.Runner{ID: "r-1"},
		},
	}
}

// TestRegression_DisplaceForSwitch_DormantChatFlaggedWorking_DoesNotWaitForever is
// the fix for a chat that could only be abandoned.
//
// switchProviderLocked's retry loop asks displaceForSwitch whether to go round
// again, and a `working` chat answered yes so the outgoing TUI is kept alive until
// a later hook restates the work level as zero. On a DORMANT chat that wait can
// never end: there is no CLI to finish the work and none to send the hook. The loop
// therefore spun forever — about one lap per awaitTurnOrForce deadline — holding
// the chat's spawn gate, which is a plain mutex with no context on it. Every later
// resume, prompt and switch on that chat queued behind it and never answered at
// all, with no access-log line either, because the log is written on completion.
//
// ResumeChat enters this same locked path, so the one call whose job is to bring a
// dormant chat back was the call the stale flag stranded: the client sat on its
// "Resuming this chat…" spinner — which carries no button — until the user gave up
// on the chat. A durable `working` outliving its CLI is a REACHABLE state, not a
// hypothetical: a SIGKILL mid-background-work sends no final stop, and boot
// reconciliation only heals chats still reachable from a live runner row.
func TestRegression_DisplaceForSwitch_DormantChatFlaggedWorking_DoesNotWaitForever(t *testing.T) {
	rs := runnersForDisplace(t, true /* working */, false /* live */)

	retry, err := rs.displaceForSwitch(context.Background(), domain.Chat{ID: "chat-1"})

	require.NoError(t, err)
	assert.False(t, retry,
		"a dormant chat's stale working flag must not ask the caller to loop again: "+
			"nothing can ever clear it, so the retry holds the chat's spawn gate forever "+
			"and every later resume on that chat blocks with no response at all")
}

// TestRegression_DisplaceForSwitch_LiveRunnerFlaggedWorking_StillWaits is the
// regression-of-the-regression-guard. The fix above must not also swallow the real
// case the retry exists for: a turn_stop that handed work to the background after
// the first await released its runner-scoped turn. There the CLI is alive, it is
// genuinely still working, and quitting it there costs the answer — so a LIVE
// runner must still be waited for exactly as before.
func TestRegression_DisplaceForSwitch_LiveRunnerFlaggedWorking_StillWaits(t *testing.T) {
	rs := runnersForDisplace(t, true /* working */, true /* live */)

	retry, err := rs.displaceForSwitch(context.Background(), domain.Chat{ID: "chat-1"})

	require.NoError(t, err)
	assert.True(t, retry,
		"a LIVE runner still doing background work must still be waited for — "+
			"there is a CLI to finish it and a hook coming to say so")
}

// A prompt steered into the turn the switch waited out is waited for too: the
// switch goes round again to its pending-prompt wait rather than refusing.
func TestDisplaceForSwitch_APromptSteeredAfterTheWaitsSendsTheSwitchRoundAgain(t *testing.T) {
	rs := runnersForDisplace(t, false /* working */, true /* live */)
	appendPrompt(t, rs.ledger, "steered", "r-1")

	retry, err := rs.displaceForSwitch(context.Background(), domain.Chat{ID: "chat-1"})

	require.NoError(t, err)
	assert.True(t, retry)
	assert.Equal(t, ledger.PromptStatusPending, promptReceipt(t, rs.ledger, "steered").Status,
		"its live carrier still owes it")
}

// TestRegression_QuitOutgoingCLI_TerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY
// mirrors TestRegression_RetireTerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY
// (lifecycle_internal_test.go): quitOutgoingCLI — SwitchProvider's own teardown of the
// chat's outgoing CLI, reached from displaceForSwitch — had the SAME gap retire() did
// before its fix. It terminated only live.TerminalSession, the ORIGINAL companion PTY
// every api-transport spawn forks alongside its connection and never reassigns; once
// SwitchToTerminal has run, that field names a different, already-abandoned process, and
// the actually-attached native-view PTY it forks separately (tracked only in rs.attached)
// kept running indefinitely — a real process leak reachable by switching provider on a
// chat while it is attached to its native terminal view, with rs.attached left answering
// AttachedTerminalSession for a runner id the switch has already moved past.
func TestRegression_QuitOutgoingCLI_TerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY(t *testing.T) {
	term := &fakeTermForAttach{}
	store := &stopRetireRunnerStore{
		runner: engineagents.Runner{ID: "runner-1", TerminalSession: "companion-pty-term"},
	}
	rs := &Runners{
		runnerStore:   store,
		attached:      newAttachRegistry(),
		apiConns:      newAPIConnRegistry(),
		term:          term,
		inflightTurns: inflight.NewTurns(),
	}
	rs.attached.set("runner-1", attachedView{termSessID: "native-view-term"})

	require.NoError(t, rs.quitOutgoingCLI(context.Background(), "chat-1"))

	assert.Contains(t, term.terminated, "companion-pty-term",
		"the original companion PTY is still a real process and must still be torn down")
	assert.Contains(t, term.terminated, "native-view-term",
		"the actually-attached native view PTY — the one the user was looking at — must be torn down too")
	_, stillAttached := rs.attached.get("runner-1")
	assert.False(t, stillAttached, "the attach registry entry must be forgotten, or it answers for a runner nothing will revisit")
	assert.True(t, store.displaced, "the outgoing runner must still be displaced from the chat")
}
