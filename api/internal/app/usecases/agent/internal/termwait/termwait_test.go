package termwait_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const (
	chatID  = "chat-1"
	wsID    = "ws-1"
	session = "pty-1"
	// trustScreen is a claude workspace-trust dialog, reduced to the line the
	// descriptor's kinded needle matches.
	trustScreen = "❯ 1. Yes, I trust this folder\n  Enter to confirm · Esc to cancel"
	idleScreen  = "> Ready.\n  shift+tab to cycle"
)

// rig is one detector plus every mock behind it, assembled in the state a HEALTHY
// blocked chat is in: one live runner, an idle chat, no pending prompts, and a
// provider that declares the trust needle. Each test then breaks exactly one of
// those and asserts what changes.
type rig struct {
	detector termwait.Detector
	runners  *fakeRunners
	chats    *fakeChats
	choices  *fakeChoices
	screens  *fakeScreens
	prompts  *fakePrompts
	rec      *recorder
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newRigEvery(t, 0)
}

// newRigEvery builds the rig with an explicit sweep interval, for the two tests
// that drive the cadence loop rather than calling Sweep directly.
func newRigEvery(t *testing.T, interval time.Duration) *rig {
	t.Helper()
	runners := &fakeRunners{live: []domain.AgentRunner{{
		ID:              "runner-1",
		WorkspaceID:     wsID,
		ProviderID:      "claude",
		TerminalSession: session,
		CurrentChatID:   chatID,
	}}}
	chats := &fakeChats{byID: map[string]domain.AgentChat{
		chatID: {ID: chatID, WorkspaceID: wsID},
	}}
	choices := &fakeChoices{pending: map[string][]domain.ActivityChoice{}}
	screens := newScreens()
	screens.set(session, trustScreen)
	prompts := &fakePrompts{needles: map[string][]engineagents.TerminalPrompt{
		"claude": {{Kind: domain.AgentTerminalWaitTrust, Needle: "I trust this folder"}},
	}}

	r := &rig{runners: runners, chats: chats, choices: choices, screens: screens, prompts: prompts, rec: &recorder{}}
	r.detector = termwait.New(termwait.Deps{
		Runners:  runners,
		Chats:    chats,
		Choices:  choices,
		Screens:  screens,
		Prompts:  prompts,
		Interval: interval,
	})
	return r
}

func (r *rig) sweep() {
	r.detector.Sweep(context.Background(), r.rec.publish)
}

func TestDetector_Sweep_ReportsABlockedChat(t *testing.T) {
	r := newRig(t)

	r.sweep()

	assert.Equal(t, domain.AgentTerminalWait{Waiting: true, Kind: domain.AgentTerminalWaitTrust},
		r.detector.Wait(chatID))
	require.Len(t, r.rec.all(), 1)
	assert.Equal(t, published{chatID, wsID, domain.AgentTerminalWait{
		Waiting: true, Kind: domain.AgentTerminalWaitTrust,
	}}, r.rec.all()[0])
}

// TestDetector_Sweep_UnrecognisedPromptCarriesNoKind is the honest-fallback path:
// the screen matched a needle that identifies nothing, so the verdict says the CLI
// is blocked and refuses to say what by. The UI's generic wording hangs off this.
func TestDetector_Sweep_UnrecognisedPromptCarriesNoKind(t *testing.T) {
	r := newRig(t)
	r.prompts.needles["claude"] = []engineagents.TerminalPrompt{{Needle: "Enter to confirm"}}

	r.sweep()

	assert.Equal(t, domain.AgentTerminalWait{Waiting: true}, r.detector.Wait(chatID))
}

// --- Gate ordering. These four are the contract. ---

// TestDetector_Sweep_WorkingChatIsNeverWaiting is gate 2. A busy agent is not
// stuck, and the spinner already says what it is doing — a "your agent is stuck"
// banner over it would be a false alarm on the commonest state a chat is ever in.
//
// It also asserts the SHORT CIRCUIT: the choices read and the screen match are
// never reached, so a working chat costs one map lookup.
func TestDetector_Sweep_WorkingChatIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID, Working: true}

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.choices.asked, "gate 2 must short-circuit before the choices read")
	assert.Zero(t, r.prompts.asked, "gate 2 must short-circuit before the screen match")
}

// TestDetector_Sweep_PendingChoiceIsNeverWaiting is gate 3, and it is the gate
// that stops this feature hijacking prompts the chat can already answer.
//
// A tool permission is a prompt the CLI paints in its terminal AND reports over a
// hook. The chat is showing its card with buttons; telling the user to go to the
// terminal instead would march them somewhere they did not need to go, for a
// question they could have answered where they were.
func TestDetector_Sweep_PendingChoiceIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.choices.pending[chatID] = []domain.ActivityChoice{{ID: "choice-1", ChatID: chatID}}

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.prompts.asked, "gate 3 must short-circuit before the screen match")
}

// TestDetector_Sweep_ProviderDeclaringNoNeedlesNeverWaits is the degradation
// guarantee: a provider with no declared prompts behaves byte-identically to how
// it did before this existed, even parked on a screen that would match another
// provider's needles.
func TestDetector_Sweep_ProviderDeclaringNoNeedlesNeverWaits(t *testing.T) {
	r := newRig(t)
	r.prompts.needles = map[string][]engineagents.TerminalPrompt{}

	r.sweep()

	assert.Equal(t, domain.AgentTerminalWait{}, r.detector.Wait(chatID))
	assert.Empty(t, r.rec.all())
}

// TestDetector_Sweep_ChatWithNoLiveRunnerIsNeverWaiting is gate 1. A dormant chat
// is not blocked on anything, because there is nothing left to block.
func TestDetector_Sweep_ChatWithNoLiveRunnerIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.runners.live = nil

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
}

// TestDetector_Sweep_RunnerWithNoPTYIsNeverWaiting covers the half of gate 1 the
// census cannot answer: a live row whose PTY id is empty has nothing to look at.
func TestDetector_Sweep_RunnerWithNoPTYIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.runners.live[0].TerminalSession = ""

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Zero(t, r.prompts.asked)
}

// TestDetector_Sweep_DisplacedRunnerNamesNoChat covers a runner Crowbar has taken
// OFF its chat while its process finishes dying. There is no conversation to say
// anything about, so it is skipped entirely rather than reported against "".
func TestDetector_Sweep_DisplacedRunnerNamesNoChat(t *testing.T) {
	r := newRig(t)
	r.runners.live[0].CurrentChatID = ""

	r.sweep()

	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.prompts.asked)
}

// --- Edges: what changes, and when it is published. ---

// TestDetector_Sweep_PublishesOnlyOnChange is what keeps a parked chat from
// flooding the feed. The banner is up; nothing about it has changed; nothing is
// sent.
func TestDetector_Sweep_PublishesOnlyOnChange(t *testing.T) {
	r := newRig(t)

	r.sweep()
	r.sweep()
	r.sweep()

	assert.Len(t, r.rec.all(), 1)
}

// TestDetector_Sweep_UnmovedScreenIsNotReRendered is the cost promise. A chat
// parked on a dialog produces no output, so every tick after the first must
// answer from the cached screen verdict without rendering the grid again.
func TestDetector_Sweep_UnmovedScreenIsNotReRendered(t *testing.T) {
	r := newRig(t)

	r.sweep()
	first := r.screens.renderCount()
	r.sweep()
	r.sweep()

	assert.Equal(t, 1, first)
	assert.Equal(t, first, r.screens.renderCount(), "an unmoved screen must not be re-rendered")
	assert.True(t, r.detector.Wait(chatID).Waiting, "and the standing verdict must survive")
}

// TestDetector_Sweep_ClearsWhenTheUserAnswersAtTheTerminal is the other edge: the
// dialog is gone from the screen, so the banner comes down and the clearing frame
// carries the workspace it belongs to.
func TestDetector_Sweep_ClearsWhenTheUserAnswersAtTheTerminal(t *testing.T) {
	r := newRig(t)
	r.sweep()

	r.screens.set(session, idleScreen)
	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	sent := r.rec.all()
	require.Len(t, sent, 2)
	assert.Equal(t, published{chatID, wsID, domain.AgentTerminalWait{}}, sent[1])
}

// TestDetector_Sweep_ClearsWhenTheRunnerDies stops a banner outliving the process
// it described. The chat leaves the live census, and a standing "waiting" verdict
// on it is explicitly cleared rather than merely forgotten — a client that only
// ever hears about changes would otherwise keep the banner up forever.
func TestDetector_Sweep_ClearsWhenTheRunnerDies(t *testing.T) {
	r := newRig(t)
	r.sweep()

	r.runners.live = nil
	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	sent := r.rec.all()
	require.Len(t, sent, 2)
	assert.Equal(t, published{chatID, wsID, domain.AgentTerminalWait{}}, sent[1])
}

// TestDetector_Sweep_DepartedChatThatWasNotWaitingIsSilent is the other half of
// that rule: forgetting an ordinary chat must not emit a frame nobody needs.
func TestDetector_Sweep_DepartedChatThatWasNotWaitingIsSilent(t *testing.T) {
	r := newRig(t)
	r.screens.set(session, idleScreen)
	r.sweep()

	r.runners.live = nil
	r.sweep()

	assert.Empty(t, r.rec.all())
}

// TestDetector_Sweep_ReplacedRunnerRereadsTheScreen guards a real trap. A provider
// switch puts a NEW PTY on the same chat, and its generation counter starts again
// — so a cached generation carried across would let the fresh screen read as
// "unchanged" and inherit the dead process's verdict.
func TestDetector_Sweep_ReplacedRunnerRereadsTheScreen(t *testing.T) {
	r := newRig(t)
	r.sweep()
	require.True(t, r.detector.Wait(chatID).Waiting)

	// The replacement's own screen is at generation 1 — the same number the old
	// PTY was last read at.
	r.runners.live[0].TerminalSession = "pty-2"
	r.screens.set("pty-2", idleScreen)
	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
}

// TestDetector_Sweep_SessionThatStopsAnsweringDropsTheVerdict covers a PTY that
// has gone while its runner row has not caught up: no screen means no evidence,
// and no evidence means no claim.
func TestDetector_Sweep_SessionThatStopsAnsweringDropsTheVerdict(t *testing.T) {
	r := newRig(t)
	r.sweep()
	require.True(t, r.detector.Wait(chatID).Waiting)

	r.screens.absent[session] = true
	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
}

// TestDetector_Sweep_ScreenCacheSurvivesABusyTurn pins the interaction between the
// gates and the cache. A chat goes busy and idle repeatedly while its screen sits
// on the same content; the busy tick must not throw away the screen answer and
// force a re-render on the far side of every turn.
func TestDetector_Sweep_ScreenCacheSurvivesABusyTurn(t *testing.T) {
	r := newRig(t)
	r.sweep()
	renders := r.screens.renderCount()

	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.sweep()
	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID}
	r.sweep()

	assert.Equal(t, renders, r.screens.renderCount())
	assert.True(t, r.detector.Wait(chatID).Waiting)
}

// --- Failure handling. Silence, never a false alarm. ---

// TestDetector_Sweep_CensusFailureChangesNothing: an empty list would clear every
// standing verdict at once and take every banner down, so a failed census leaves
// the whole picture exactly as it was.
func TestDetector_Sweep_CensusFailureChangesNothing(t *testing.T) {
	r := newRig(t)
	r.sweep()

	r.runners.err = errBoom
	r.sweep()

	assert.True(t, r.detector.Wait(chatID).Waiting)
	assert.Len(t, r.rec.all(), 1)
}

func TestDetector_Sweep_ChatReadFailureIsSilent(t *testing.T) {
	r := newRig(t)
	r.chats.err = errBoom

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.prompts.asked)
}

func TestDetector_Sweep_ChoiceReadFailureIsSilent(t *testing.T) {
	r := newRig(t)
	r.choices.err = errBoom

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Zero(t, r.prompts.asked)
}

// TestDetector_Sweep_NilPublisherIsSafe: the sweep is usable without a fan-out, so
// a caller that only wants the standing answer need not invent one.
func TestDetector_Sweep_NilPublisherIsSafe(t *testing.T) {
	r := newRig(t)

	r.detector.Sweep(context.Background(), nil)

	assert.True(t, r.detector.Wait(chatID).Waiting)
}

func TestDetector_Wait_UnknownChatIsNotWaiting(t *testing.T) {
	r := newRig(t)

	assert.Equal(t, domain.AgentTerminalWait{}, r.detector.Wait("never-heard-of-it"))
}

// --- The cadence. ---

// TestDetector_Run_SweepsImmediatelyAndThenOnTheClock pins the first-tick rule: a
// daemon restarting under a CLI already parked on a dialog must not show nothing
// for a whole interval, which is precisely the state this exists to end.
//
// It blocks on the publisher, never on a clock.
func TestDetector_Run_SweepsImmediatelyAndThenOnTheClock(t *testing.T) {
	r := newRigEvery(t, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan domain.AgentTerminalWait, 4)
	r.detector.Run(ctx, func(_, _ string, wait domain.AgentTerminalWait) {
		select {
		case got <- wait:
		default:
		}
	})

	select {
	case wait := <-got:
		assert.True(t, wait.Waiting)
	case <-time.After(5 * time.Second):
		t.Fatal("the first sweep must run immediately, not one interval in")
	}

	// The loop keeps going: clearing the dialog must reach the publisher too.
	r.screens.set(session, idleScreen)
	select {
	case wait := <-got:
		assert.False(t, wait.Waiting)
	case <-time.After(5 * time.Second):
		t.Fatal("the cadence stopped after its first pass")
	}
}

// TestDetector_Run_StopsWithItsContext proves the loop is owned by the caller's
// lifetime rather than by the process's.
//
// Deterministic, with no waiting for something NOT to happen: the interval is an
// hour and the context is already cancelled, so the loop takes its immediate sweep
// and then has exactly one ready case to select. Observing the publish establishes
// happens-before on the counter, and nothing writes it again.
func TestDetector_Run_StopsWithItsContext(t *testing.T) {
	r := newRigEvery(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	swept := make(chan struct{}, 1)
	r.detector.Run(ctx, func(string, string, domain.AgentTerminalWait) {
		swept <- struct{}{}
	})
	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("the immediate sweep must run even on a context that is already done")
	}

	assert.Equal(t, 1, r.runners.calls, "the loop must not tick again after its context is done")
}

// TestDetector_Run_DefaultsItsInterval covers the zero-interval fallback: a caller
// that names no cadence gets DefaultInterval rather than a ticker of zero, which
// panics. Asserted through the loop itself — the immediate sweep still lands, and
// the tick that would follow is two seconds away and never observed.
func TestDetector_Run_DefaultsItsInterval(t *testing.T) {
	r := newRigEvery(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	swept := make(chan struct{}, 1)
	r.detector.Run(ctx, func(string, string, domain.AgentTerminalWait) {
		swept <- struct{}{}
	})

	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("a detector with no declared interval must still sweep")
	}
	assert.Equal(t, termwait.DefaultInterval, 2*time.Second)
}
