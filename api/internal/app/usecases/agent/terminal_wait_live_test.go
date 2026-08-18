package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// This file wires the REAL halves of the detector together over a REAL PTY: a
// process paints the trust dialog, the daemon's own VT model takes it, the
// terminal engine renders that model to text, and the SHIPPED claude descriptor's
// needles are matched against it.
//
// It exists because every other test of this feature stubs one of those seams,
// and the seams are where it would actually break — a VT model that renders the
// box drawing differently, a screen read that arrives blank, a needle that no
// longer survives the wrapping. Only the four gate inputs that are ordinary
// database reads (the runner census, the chat's busy state, its pending prompts)
// are faked here.
//
// WHAT IT IS NOT: a live claude. Spawning a real one would make it write a trust
// decision into the user's own ~/.claude.json, keyed by the directory — a
// provider home this work is forbidden to touch — so the screen is painted by a
// shell instead, from the capture recorded in
// tests/integration/agent/barriers_test.go.

// liveTrustScreen is the workspace-trust dialog exactly as claude 2.1.207 paints
// it, from the capture recorded in tests/integration/agent/barriers_test.go. Fed
// through a real PTY so the model does the wrapping, padding and box drawing for
// real rather than the test asserting against its own string.
const liveTrustScreen = `╭─────────────────────────────────────────────╮
│ Do you trust the files in this folder?      │
│                                             │
│ ❯ 1. Yes, I trust this folder               │
│   2. No, exit                               │
│                                             │
│ Enter to confirm · Esc to cancel            │
╰─────────────────────────────────────────────╯
`

// liveRig is the detector over a real terminal engine and the real agents engine.
type liveRig struct {
	detector termwait.Detector
	engine   engineterminal.Engine
	chat     domain.AgentChat
	pending  []domain.ActivityChoice
	runners  []domain.AgentRunner
}

func (r *liveRig) AllLive(context.Context) ([]domain.AgentRunner, error) { return r.runners, nil }

func (r *liveRig) GetChat(context.Context, string) (domain.AgentChat, error) { return r.chat, nil }

func (r *liveRig) PendingChoices(context.Context, string) ([]domain.ActivityChoice, error) {
	return r.pending, nil
}

// MatchTerminalPrompt is the REAL descriptor lookup, resolved from the embedded
// catalogue exactly as the usecase resolves it — no override dir, so a machine
// with a customised descriptor cannot make this test lie.
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

// newLiveRig spawns a real PTY that paints `screen` and then holds it, and points
// a detector at it through the real terminal engine.
func newLiveRig(t *testing.T, screen string) *liveRig {
	t.Helper()
	engine := engineterminal.New()
	t.Cleanup(func() { engine.Shutdown() })

	// `cat` on a fifo would need cleanup a failed test could skip; a read that
	// never completes holds the process open with nothing to tear down but the
	// PTY itself, which Shutdown takes.
	sessionID, err := engine.CreateCommand(
		context.Background(),
		"ws-live",
		t.TempDir(),
		[]string{"/bin/sh", "-c", "printf '%s' \"$SCREEN\"; read -r _"},
		append(os.Environ(), "SCREEN="+screen),
		func() {},
	)
	require.NoError(t, err)

	rig := &liveRig{
		engine: engine,
		chat:   domain.AgentChat{ID: "chat-live", WorkspaceID: "ws-live"},
		runners: []domain.AgentRunner{{
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

// awaitVerdict sweeps until pred holds. The wait is on the SCREEN — the PTY's
// output arrives when the process gets around to writing it — and each attempt is
// a real evaluation rather than a clock reading.
func (r *liveRig) awaitVerdict(t *testing.T, what string, pred func(domain.AgentTerminalWait) bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		r.detector.Sweep(context.Background(), nil)
		return pred(r.detector.Wait("chat-live"))
	}, 20*time.Second, 20*time.Millisecond, what)
}

// TestDetector_RealPTY_ReportsTheTrustDialog is the end-to-end proof: a real
// process paints the real dialog, and the daemon works out — from its own screen
// model and the shipped descriptor — both THAT the CLI is blocked and WHAT by.
func TestDetector_RealPTY_ReportsTheTrustDialog(t *testing.T) {
	rig := newLiveRig(t, liveTrustScreen)

	rig.awaitVerdict(t, "the trust dialog is recognised", func(w domain.AgentTerminalWait) bool {
		return w.Waiting && w.Kind == domain.AgentTerminalWaitTrust
	})
}

// TestDetector_RealPTY_WorkingChatIsNeverWaiting is gate 2 over the same real
// screen. The dialog is genuinely on the PTY; the chat says it is busy; nothing is
// reported. Stated against real output because this is the false-alarm case that
// would appear on healthy chats constantly if the ordering were wrong.
func TestDetector_RealPTY_WorkingChatIsNeverWaiting(t *testing.T) {
	rig := newLiveRig(t, liveTrustScreen)
	rig.awaitVerdict(t, "the dialog reaches the screen", func(w domain.AgentTerminalWait) bool {
		return w.Waiting
	})

	rig.chat.Working = true
	rig.detector.Sweep(context.Background(), nil)

	assert.False(t, rig.detector.Wait("chat-live").Waiting)
}

// TestDetector_RealPTY_PendingChoiceIsNeverWaiting is gate 3 over real output: a
// prompt the chat can already answer suppresses this entirely, so an ordinary tool
// permission never turns into "your agent is stuck in the terminal".
func TestDetector_RealPTY_PendingChoiceIsNeverWaiting(t *testing.T) {
	rig := newLiveRig(t, liveTrustScreen)
	rig.awaitVerdict(t, "the dialog reaches the screen", func(w domain.AgentTerminalWait) bool {
		return w.Waiting
	})

	rig.pending = []domain.ActivityChoice{{ID: "choice-1", ChatID: "chat-live"}}
	rig.detector.Sweep(context.Background(), nil)

	assert.False(t, rig.detector.Wait("chat-live").Waiting)
}

// TestDetector_RealPTY_OrdinaryOutputIsNotABlock is the false-positive guard where
// it matters most: a working agent's real screen, through the real model, against
// the real needles.
func TestDetector_RealPTY_OrdinaryOutputIsNotABlock(t *testing.T) {
	rig := newLiveRig(t, "> Ready. Try \"fix the failing test\"\n  shift+tab to cycle · ? for shortcuts\n")

	// Sweep until the screen has demonstrably arrived, so this is a real negative
	// rather than a verdict taken before the process wrote anything.
	require.Eventually(t, func() bool {
		text, _, _ := rig.engine.Screen(rig.runners[0].TerminalSession, 0)
		return len(text) > 0
	}, 20*time.Second, 20*time.Millisecond, "the shell painted nothing")
	rig.detector.Sweep(context.Background(), nil)

	assert.False(t, rig.detector.Wait("chat-live").Waiting)
}
