package runner_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The production terminal engine renders screens, which is what decides whether a
// daemon has a detector at all. Asserting it here means a change to the engine's
// surface fails at compile time rather than by silently switching the sweep off.
var _ termwait.Screens = engineterminal.Engine(nil)

// plainCommander is a PTY seam that cannot render a screen — the shape a headless
// or stubbed terminal has.
type plainCommander struct{}

func (plainCommander) CreateCommand(
	context.Context, string, string, []string, []string, func(),
) (string, error) {
	return "", nil
}

func (plainCommander) TerminateGraceful(context.Context, string) error { return nil }

func (plainCommander) SessionLive(context.Context, string) bool { return false }

// screenReadingCommander is the same seam WITH a screen, which is what the
// production terminal engine has.
type screenReadingCommander struct{ plainCommander }

func (screenReadingCommander) Screen(string, uint64) (string, uint64, bool) {
	return "", 0, false
}

// stubTurns satisfies the hook-ingress port with the cheapest possible answers.
// Nothing here is exercised; SetTurns only has to be able to bind it.
type stubTurns struct{}

func (stubTurns) IngestHook(context.Context, string, string, string, []byte) error { return nil }

func (stubTurns) ReplayStartupHook(string, inflight.Hook) {}

func (stubTurns) AwaitTurnComplete(context.Context, string) error { return nil }

func (stubTurns) ChatWorking(context.Context, string) (bool, error) { return false, nil }

func (stubTurns) RecordStop(context.Context, string) error { return nil }

func (stubTurns) RecordChatSwitch(context.Context, string, string, string) error { return nil }

func (stubTurns) SetMessageDelta(func(chatID, workspaceID, messageID, text string)) {}

func (stubTurns) SetCompactionStatus(func(chatID, workspaceID string, active bool)) {}

func (stubTurns) MatchTerminalPrompt(
	context.Context, string, string,
) (engineagents.TerminalPrompt, bool) {
	return engineagents.TerminalPrompt{}, false
}

func (stubTurns) MatchTerminalNotice(
	context.Context, string, string,
) (engineagents.TerminalNotice, bool) {
	return engineagents.TerminalNotice{}, false
}

func (stubTurns) OpenWork(context.Context, string) (bool, error) { return false, nil }

func (stubTurns) UnfinishedSince(string) (time.Time, bool) { return time.Time{}, false }

func (stubTurns) AbandonMessage(context.Context, string) (bool, error) { return false, nil }

func (stubTurns) AbandonMessageForRunner(
	context.Context, string, engineagents.Runner,
) (bool, error) {
	return false, nil
}

func (stubTurns) CloseStalledTurn(context.Context, seam.Stall) {}

// A terminal that cannot render a screen leaves the daemon with NO detector, and
// every chat must then report the zero verdict rather than panicking on it.
func TestTerminalWait_WithoutADetectorIsNotWaiting(t *testing.T) {
	t.Parallel()

	rs := runner.New(runner.Deps{Terminal: plainCommander{}})
	rs.SetTurns(stubTurns{})

	assert.False(t, rs.TerminalWait("any-chat").Waiting)

	// And the sweep is a no-op rather than a nil dereference.
	rs.StartTerminalWaitSweep(t.Context(), nil, nil, nil, nil)
}

// A terminal that CAN render a screen gets a detector, built by SetTurns because
// half the detector's ports belong to the hook ingress.
func TestTerminalWait_ReadsThroughTheDetector(t *testing.T) {
	t.Parallel()

	rs := runner.New(runner.Deps{Terminal: screenReadingCommander{}})
	rs.SetTurns(stubTurns{})

	assert.Equal(t, domain.AgentTerminalWait{}, rs.TerminalWait("chat-1"),
		"a chat the sweep has never visited is not waiting")
}

// StartTerminalWaitSweep wires the publish callbacks BEFORE it checks for a
// detector. A daemon with no detector still streams assistant messages to its
// chat UI, and dropping messageDelta on that path is invisible until a user
// watches a message that never grows.
func TestStartTerminalWaitSweep_WiresMessageDeltaEvenWithNoDetector(t *testing.T) {
	t.Parallel()

	turns := &deltaRecordingTurns{}
	rs := runner.New(runner.Deps{Terminal: plainCommander{}})
	rs.SetTurns(turns)

	rs.StartTerminalWaitSweep(t.Context(), nil, nil, func(_, _, _, _ string) {}, nil)

	require.True(t, turns.wired, "a daemon with no detector still has messages to stream")
}

type deltaRecordingTurns struct {
	stubTurns
	wired bool
}

func (d *deltaRecordingTurns) SetMessageDelta(fn func(chatID, workspaceID, messageID, text string)) {
	d.wired = fn != nil
}

// StartTerminalWaitSweep wires compactionStatus BEFORE it checks for a
// detector too, for the same reason messageDelta is: a daemon with no
// detector still needs to tell its chat UI a compaction is in progress, and
// dropping the wiring on that path is invisible until a user watches a
// compaction that never shows.
func TestStartTerminalWaitSweep_WiresCompactionStatusEvenWithNoDetector(t *testing.T) {
	t.Parallel()

	turns := &compactionRecordingTurns{}
	rs := runner.New(runner.Deps{Terminal: plainCommander{}})
	rs.SetTurns(turns)

	rs.StartTerminalWaitSweep(t.Context(), nil, nil, nil, func(_, _ string, _ bool) {})

	require.True(t, turns.wired, "a daemon with no detector still has compaction status to publish")
}

type compactionRecordingTurns struct {
	stubTurns
	wired bool
}

func (c *compactionRecordingTurns) SetCompactionStatus(fn func(chatID, workspaceID string, active bool)) {
	c.wired = fn != nil
}

// ─── from termwait_live_test.go (real PTY) ────────────────────

const liveTrustScreen = `╭─────────────────────────────────────────────╮
│ Do you trust the files in this folder?      │
│                                             │
│ ❯ 1. Yes, I trust this folder               │
│   2. No, exit                               │
│                                             │
│ Enter to confirm · Esc to cancel            │
╰─────────────────────────────────────────────╯
`

type liveRig struct {
	detector termwait.Detector
	engine   engineterminal.Engine
	chat     domain.Chat
	pending  []domain.ActivityChoice
	runners  []engineagents.Runner
}

func (r *liveRig) AllLive(context.Context) ([]engineagents.Runner, error) { return r.runners, nil }

func (r *liveRig) GetChat(context.Context, string) (domain.Chat, error) { return r.chat, nil }

func (r *liveRig) PendingChoices(context.Context, string) ([]domain.ActivityChoice, error) {
	return r.pending, nil
}

func (r *liveRig) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	a, err := engineagents.New().Get(ctx, "", providerID)
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	return a.MatchTerminalPrompt(screen)
}

func newLiveRig(t *testing.T, screen string) *liveRig {
	t.Helper()
	engine := engineterminal.New()
	t.Cleanup(func() { engine.Shutdown() })

	sessionID, err := engine.CreateCommand(
		context.Background(),
		"chat-live",
		t.TempDir(),
		[]string{"/bin/sh", "-c", "printf '%s' \"$SCREEN\"; read -r _"},
		append(os.Environ(), "SCREEN="+screen),
		func() {},
	)
	require.NoError(t, err)

	rig := &liveRig{
		engine: engine,
		chat:   domain.Chat{ID: "chat-live", WorkspaceID: "ws-live"},
		runners: []engineagents.Runner{{
			ID:              "runner-live",
			WorkspaceID:     "ws-live",
			ProviderID:      "claude",
			TerminalSession: sessionID,
			CurrentChatID:   "chat-live",
		}},
	}
	rig.detector = termwait.New(termwait.Deps{
		Runners: rig,
		Chats:   rig,
		Choices: rig,
		Screens: engine,
		Prompts: rig,
	})
	return rig
}

func (r *liveRig) awaitVerdict(t *testing.T, what string, pred func(domain.AgentTerminalWait) bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		r.detector.Sweep(context.Background(), nil)
		return pred(r.detector.Wait("chat-live"))
	}, 20*time.Second, 20*time.Millisecond, what)
}

func TestDetector_RealPTY_ReportsTheTrustDialog(t *testing.T) {
	rig := newLiveRig(t, liveTrustScreen)

	rig.awaitVerdict(t, "the trust dialog is recognised", func(w domain.AgentTerminalWait) bool {
		return w.Waiting && w.Kind == domain.AgentTerminalWaitTrust
	})
}

func TestDetector_RealPTY_WorkingChatIsNeverWaiting(t *testing.T) {
	rig := newLiveRig(t, liveTrustScreen)
	rig.awaitVerdict(t, "the dialog reaches the screen", func(w domain.AgentTerminalWait) bool {
		return w.Waiting
	})

	rig.chat.Working = true
	rig.detector.Sweep(context.Background(), nil)

	assert.False(t, rig.detector.Wait("chat-live").Waiting)
}

func TestDetector_RealPTY_PendingChoiceIsNeverWaiting(t *testing.T) {
	rig := newLiveRig(t, liveTrustScreen)
	rig.awaitVerdict(t, "the dialog reaches the screen", func(w domain.AgentTerminalWait) bool {
		return w.Waiting
	})

	rig.pending = []domain.ActivityChoice{{ID: "choice-1", ChatID: "chat-live"}}
	rig.detector.Sweep(context.Background(), nil)

	assert.False(t, rig.detector.Wait("chat-live").Waiting)
}

func TestDetector_RealPTY_OrdinaryOutputIsNotABlock(t *testing.T) {
	rig := newLiveRig(t, "> Ready. Try \"fix the failing test\"\n  shift+tab to cycle · ? for shortcuts\n")

	require.Eventually(t, func() bool {
		text, _, _ := rig.engine.Screen(rig.runners[0].TerminalSession, 0)
		return len(text) > 0
	}, 20*time.Second, 20*time.Millisecond, "the shell painted nothing")
	rig.detector.Sweep(context.Background(), nil)

	assert.False(t, rig.detector.Wait("chat-live").Waiting)
}
