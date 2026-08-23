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
	chat     domain.AgentChat
	pending  []domain.ActivityChoice
	runners  []domain.AgentRunner
}

func (r *liveRig) AllLive(context.Context) ([]domain.AgentRunner, error) { return r.runners, nil }

func (r *liveRig) GetChat(context.Context, string) (domain.AgentChat, error) { return r.chat, nil }

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
