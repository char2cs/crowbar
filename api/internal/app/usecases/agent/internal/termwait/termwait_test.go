package termwait_test

import (
	"context"
	"strconv"
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

	// usageLimitScreen is the REAL codex-cli 0.146.0 screen from the capture that
	// found this defect — the banner as it wrapped across three rows at 100
	// columns, not a banner somebody wrote to make a test pass. A synthetic
	// fixture here would be the same mistake this repo has made before: it would
	// have hidden the wrapping, which is precisely the part the capture rule has
	// to get right.
	usageLimitScreen = "■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit\n" +
		"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026\n" +
		"12:30 PM."
	usageLimitNeedle = "You've hit your usage limit"
	// usageLimitSentence is that same banner with the TERMINAL'S row breaks taken
	// back out — what the provider wrote, and what the chat shows.
	usageLimitSentence = "■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit " +
		"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026 " +
		"12:30 PM."
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
	notices  *fakeNotices
	work     *fakeWork
	deliv    *fakeDeliveries
	clock    *clock
	rec      *recorder
	stalls   *stalls
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
	notices := &fakeNotices{needles: map[string][]engineagents.TerminalNotice{
		"codex": {{
			Kind:     engineagents.TerminalNoticeUsageLimit,
			Needle:   usageLimitNeedle,
			Text:     usageLimitSentence,
			EndsTurn: true,
		}},
	}}

	r := &rig{
		runners: runners, chats: chats, choices: choices, screens: screens,
		prompts: prompts, notices: notices, work: &fakeWork{}, clock: newClock(),
		rec: &recorder{}, stalls: &stalls{},
		deliv: &fakeDeliveries{pending: map[string]termwait.Delivery{}},
	}
	r.detector = termwait.New(termwait.Deps{
		Runners:    runners,
		Chats:      chats,
		Choices:    choices,
		Screens:    screens,
		Prompts:    prompts,
		Notices:    notices,
		Work:       r.work,
		OnStall:    r.stalls.onStall,
		Deliveries: r.deliv,
		Interval:   interval,
		Now:        r.clock.Now,
	})
	return r
}

// wedged puts the rig in the state defect 1 was measured in: a codex runner, a
// chat that IS Working, and the usage-limit banner on screen. Nothing has been
// quiet long enough yet — that is each test's own business.
func (r *rig) wedged() {
	r.runners.live[0].ProviderID = "codex"
	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.screens.set(session, usageLimitScreen)
}

// delivering puts the rig in the state defect 2 was measured in: an idle chat at
// its own composer, with one prompt still open in the delivery journal against the
// runner that received it.
func (r *rig) delivering() {
	r.screens.set(session, idleScreen)
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-1"}
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
// The screen IS read for a working chat — that read is what the stall question is
// built on, and one read answers both — but the verdict it feeds is forced to
// zero here regardless of what the screen says. What still short-circuits is the
// repository read: a working chat costs no database query until every in-memory
// stall gate has already agreed, which on this one they have not.
func TestDetector_Sweep_WorkingChatIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID, Working: true}

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.choices.asked, "a busy tick must not reach the choices read")
	assert.Zero(t, r.work.asked, "a busy tick must not reach the open-work read")
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

// --- The stall question. Defect 1: a failed turn wedges the spinner forever. ---
//
// Every test below moves an INJECTED CLOCK and calls Sweep. Nothing sleeps, and
// nothing waits on a real duration — which is the only way a 120-second rule can
// be tested at all, and the only way these can never be flaky.

// TestRegression_StalledTurnIsClosedWhenTheScreenHasBeenQuiet is defect 1.
//
// Measured against codex-cli 0.146.0: the user sent a prompt, user_prompt fired,
// the turn opened, codex hit its usage limit, painted its banner and STAYED
// ALIVE. No Stop hook, so nothing closed the turn; no exit, so the runner-exit
// reconcile never ran either. The chat spun for 44 minutes (turn_started
// 16:26:34Z, turn_stopped 17:10:24Z) and only stopped because a human switched
// provider.
//
// This is that chat: a live runner, Working true, no pending choice, nothing open
// in the record, and the measured banner sitting still on the screen. Once the
// quiet period has elapsed the turn is reported for closing.
func TestRegression_StalledTurnIsClosedWhenTheScreenHasBeenQuiet(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep() // starts the quiet clock at the generation the banner arrived on
	require.Empty(t, r.stalls.all(), "nothing may close on the tick that first sees the banner")

	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	got := r.stalls.all()
	require.Len(t, got, 1)
	assert.Equal(t, chatID, got[0].ChatID)
	assert.Equal(t, wsID, got[0].WorkspaceID)
	assert.Equal(t, "codex", got[0].ProviderID)
	assert.Equal(t, "runner-1", got[0].RunnerID)
	assert.Equal(t, session, got[0].SessionID)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, got[0].Notice.Kind)
}

// TestRegression_StalledTurnIsNotClosedBeforeTheQuietPeriod is the other half of
// the same regression, and the reason the rule is a conjunction rather than a
// needle match: a banner on screen is not on its own evidence that the CLI has
// stopped. One second short of the period, nothing happens.
func TestRegression_StalledTurnIsNotClosedBeforeTheQuietPeriod(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet - time.Second)
	r.sweep()

	assert.Empty(t, r.stalls.all())
	assert.Zero(t, r.work.asked, "the repository gates must not be reached before the clock elapses")
}

// TestDetector_Sweep_MovingScreenIsNeverClosed IS THE INVARIANT. THE SPINNER MUST
// NEVER GO DARK WHILE THE AGENT IS GENUINELY WORKING.
//
// The screen advances on EVERY sweep — the shape a working CLI was measured to
// have (claude 2.1.234 mid-generation: continuous model writes, never a
// two-second window with none) — while the matching end-of-turn notice sits on it
// the whole time, which is the worst case: the corroborating signal is present
// and only the quiet clock is holding the line. It runs for FIFTY TIMES the quiet
// period, and the turn is never closed.
//
// Any change at all resets the clock to zero. If this test ever fails, the
// feature is closing turns under working agents and must be reverted, not tuned.
func TestDetector_Sweep_MovingScreenIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	previous := usageLimitScreen

	for i := 0; i < 50; i++ {
		r.clock.advance(termwait.DefaultStallQuiet)
		// A working CLI's screen changes TEXT, not merely its generation — the
		// spinner row carries an elapsed-time counter — so each pass here writes a
		// genuinely different string. That distinction is load-bearing now that
		// the clock is measured on the rendered text: a fake that only bumped the
		// generation would make this test pass vacuously.
		frame := usageLimitScreen + "\n  working (" + strconv.Itoa(i) + "s)"
		require.NotEqual(t, previous, frame, "the fixture must move the TEXT, not just the generation")
		previous = frame
		r.screens.set(session, frame)
		r.sweep()

		require.Empty(t, r.stalls.all(), "a moving screen must never close a turn")
		require.True(t, r.chats.byID[chatID].Working, "and the chat must still be working")
	}
}

// TestDetector_Sweep_ProviderDeclaringNoNoticesNeverCloses is the degradation
// guarantee for the stall half, and it is what claude relies on: a provider that
// declares no notice behaves byte-for-byte as it did before this existed, even
// parked forever on a screen that would match another provider's needles.
func TestDetector_Sweep_ProviderDeclaringNoNoticesNeverCloses(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.runners.live[0].ProviderID = "claude" // declares prompts, declares no notices

	r.sweep()
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_SessionWithNoScreenNeverCloses: gen == 0 is the engine saying
// it cannot answer for this session at all — dead, unknown, or a suspended
// placeholder. No screen is NO EVIDENCE, and no evidence closes nothing, however
// long it goes on for.
//
// The banner is read FIRST and the session goes dark afterwards, which is what
// makes this test bite: a detector that answered from its cache when the engine
// stopped answering would have every gate satisfied — a matching notice, a
// generation that cannot move, and therefore a quiet period that runs forever —
// and would close the turn on a session it can no longer see.
//
// (For the case this whole feature exists to fix the branch is unreachable: the
// terminal engine's maintenance sweep skips agentic CLI sessions in both phases,
// so a live agent runner is never suspended. It stays because it is still the
// right answer for a session that is genuinely dead or unknown.)
func TestDetector_Sweep_SessionWithNoScreenNeverCloses(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()
	require.Equal(t, 1, r.notices.asked, "the banner must have been read and cached")

	r.screens.absent[session] = true
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	r.sweep()
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_IdleChatIsNeverClosed is the "something to close" gate. A
// chat that is not Working has no open turn, so closing one would be closing
// nothing — and it is the chat state the WAIT question is asked about instead.
func TestDetector_Sweep_IdleChatIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID}

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_PendingChoiceIsNeverClosed: a chat blocked on a prompt is
// waiting on the HUMAN, and its Working is honest. Closing its turn would take
// the question away while it was still being asked.
func TestDetector_Sweep_PendingChoiceIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.choices.pending[chatID] = []domain.ActivityChoice{{ID: "choice-1", ChatID: chatID}}

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_OpenToolCallIsNeverClosed is the gate that does not look at
// the screen at all, and it is what covers the provider whose painting behaviour
// while working has never been measured: codex was rate-limited for the whole
// probe, so "a working codex keeps painting" is an assumption there, not a
// measurement.
//
// A tool call open in the conversation record is the provider's own hook evidence
// that it is working — tool_pre arrived, tool_post has not — and that outranks a
// still screen. The chat closes only once the record says the work is done.
func TestDetector_Sweep_OpenToolCallIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.work.open = true

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	require.Empty(t, r.stalls.all(), "a chat mid-tool is working, whatever its screen shows")

	// The tool completes. Nothing about the screen changed, so the quiet clock has
	// kept running — and now the last gate agrees too.
	r.work.open = false
	r.sweep()

	assert.Len(t, r.stalls.all(), 1)
}

// TestDetector_Sweep_OpenWorkReadFailureIsSilent: a gate that cannot be read is a
// gate that has not agreed. Silence, never a close.
func TestDetector_Sweep_OpenWorkReadFailureIsSilent(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.work.err = errBoom

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_StallIsReportedOnce is the latch. Working is read from an
// asynchronously folded projection, so a chat whose turn was just closed can
// still read Working for a tick or two — and without the latch each of those
// ticks would close it again and append another notice to the conversation.
func TestDetector_Sweep_StallIsReportedOnce(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	r.sweep()

	assert.Len(t, r.stalls.all(), 1)
}

// TestDetector_Sweep_NoticeWithoutEndsTurnNeverCloses: ends_turn is the entire
// claim that a notice is evidence of anything. A message that does not declare it
// says nothing about whether the CLI is working, so it can corroborate nothing.
func TestDetector_Sweep_NoticeWithoutEndsTurnNeverCloses(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.notices.needles["codex"] = []engineagents.TerminalNotice{{
		Kind:   engineagents.TerminalNoticeUsageLimit,
		Needle: usageLimitNeedle,
		Text:   usageLimitSentence,
	}}

	r.sweep()
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_BannerScrollingAwayWithdrawsTheEvidence: the notice is cached
// against the generation it was read at, so a screen that moves replaces it —
// including replacing it with nothing. A banner that has scrolled out of the
// viewport cannot go on closing turns from memory.
func TestDetector_Sweep_BannerScrollingAwayWithdrawsTheEvidence(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.screens.set(session, idleScreen)
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

// TestDetector_Sweep_ReplacedRunnerRestartsTheQuietClock guards the same trap the
// wait verdict guards: a new PTY on the same chat has its own generation counter,
// so a clock carried across would credit the replacement with the dead process's
// stillness and close its turn on its first tick.
func TestDetector_Sweep_ReplacedRunnerRestartsTheQuietClock(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)

	r.runners.live[0].TerminalSession = "pty-2"
	r.screens.set("pty-2", usageLimitScreen)
	r.sweep()

	assert.Empty(t, r.stalls.all(), "a fresh PTY's clock starts at zero")

	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	assert.Len(t, r.stalls.all(), 1)
}

// TestDetector_Sweep_WithoutStallDependenciesNothingCloses is the wiring-level
// degradation contract: a detector built with only the wait half — which is every
// caller that predates this — answers the wait question exactly as it always did
// and never closes anything.
func TestDetector_Sweep_WithoutStallDependenciesNothingCloses(t *testing.T) {
	r := newRig(t)
	r.wedged()
	waitOnly := termwait.New(termwait.Deps{
		Runners: r.runners,
		Chats:   r.chats,
		Choices: r.choices,
		Screens: r.screens,
		Prompts: r.prompts,
		Now:     r.clock.Now,
	})

	waitOnly.Sweep(context.Background(), r.rec.publish)
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	waitOnly.Sweep(context.Background(), r.rec.publish)

	assert.Empty(t, r.stalls.all())
	assert.Zero(t, r.notices.asked)
}

// --- The quiet clock measures TEXT, not the generation counter. ---

// TestDetector_Sweep_IdenticalRepaintDoesNotResetTheQuietClock is the property
// that stops this fix being silently useless.
//
// The generation bumps on ANY consumed PTY chunk, including one that repaints
// byte-identical cells. A TUI that redraws its own chrome on a timer would
// therefore look like movement on every tick — and if its repaint period were
// shorter than the quiet window, the clock would reset forever and the turn would
// never close, with the fix installed and every other test passing.
//
// The screen is repainted four times across the window and the turn still closes
// on EXACTLY the deadline it would have had if nothing had arrived at all.
func TestDetector_Sweep_IdenticalRepaintDoesNotResetTheQuietClock(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()
	renders := r.screens.renderCount()

	for i := 0; i < 4; i++ {
		r.clock.advance(termwait.DefaultStallQuiet / 4)
		r.screens.repaint(session)
		r.sweep()
	}

	assert.Len(t, r.stalls.all(), 1, "an identical repaint is not movement")
	assert.Greater(t, r.screens.renderCount(), renders,
		"the repaints must really have been rendered — otherwise this proves nothing")
	assert.Equal(t, 1, r.notices.asked,
		"and identical text must not be re-matched: the previous answer still holds")
}

// TestDetector_Sweep_OneCharacterOfNewTextResetsTheQuietClock is the other half,
// and it is the asymmetry that must never be optimised away. A working agent
// changes TEXT, not merely bytes — claude's spinner carries its own elapsed-time
// counter — so a single character of difference is a live CLI and the clock goes
// back to zero.
func TestDetector_Sweep_OneCharacterOfNewTextResetsTheQuietClock(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.clock.advance(termwait.DefaultStallQuiet - time.Second)
	r.screens.set(session, usageLimitScreen+".")
	r.sweep()

	r.clock.advance(time.Second)
	r.sweep()
	assert.Empty(t, r.stalls.all(), "one changed character restarts the whole window")

	// And the clock really is running again rather than stopped: the full window
	// from the change closes it.
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	assert.Len(t, r.stalls.all(), 1)
}

// TestDetector_Sweep_CursorMovementCannotResetTheClock states, where it is relied
// on, a property that is inherited rather than implemented here: Screen renders
// the viewport as CONTENT ONLY — no SGR, no cursor, no scrollback
// (model.ScreenReader) — so a moved cursor is not in the string being compared
// and cannot appear as a change.
//
// Expressed as a repaint whose text is identical, because that is exactly what a
// cursor-only move looks like by the time it reaches this package. If Screen ever
// started including the cursor, this is the test that would start failing.
func TestDetector_Sweep_CursorMovementCannotResetTheClock(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	for i := 0; i < 10; i++ {
		r.clock.advance(termwait.DefaultStallQuiet / 10)
		r.screens.repaint(session)
		r.sweep()
	}

	assert.Len(t, r.stalls.all(), 1)
}

// TestDetector_Sweep_SettlesADeliveryThatProducedNoTurn is the wedge this gate
// exists for. A provider built-in — /compact, measured against claude 2.1.236 —
// is handled inside the CLI and fires neither UserPromptSubmit nor Stop, so the
// journal record waited on an acknowledgement that was never coming and the chat
// refused every later prompt for the rest of the runner's life.
func TestDetector_Sweep_SettlesADeliveryThatProducedNoTurn(t *testing.T) {
	r := newRig(t)
	r.delivering()

	r.sweep()
	assert.Empty(t, r.deliv.allSettled(), "the quiet clock has not started yet")

	r.clock.advance(termwait.DefaultDeliveryQuiet)
	r.sweep()

	assert.Equal(t, []string{"req-1"}, r.deliv.allSettled())
}

// The screen having stopped is only evidence once it has stopped for the whole
// window. One tick short must decide nothing.
func TestDetector_Sweep_LeavesADeliveryAloneUntilTheScreenHasBeenStillLongEnough(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.sweep()

	r.clock.advance(termwait.DefaultDeliveryQuiet - time.Second)
	r.sweep()

	assert.Empty(t, r.deliv.allSettled())
}

// TestDetector_Sweep_NeverSettlesADeliveryBehindABlockingModal is the gate that
// carries the argument. A CLI parked on the trust dialog is perfectly still and
// perfectly innocent: it has NOT consumed the prompt, it is waiting for a human,
// and it will read its argv the moment somebody answers. Retiring the delivery
// there drops the barrier protecting a prompt that is still going to arrive.
func TestDetector_Sweep_NeverSettlesADeliveryBehindABlockingModal(t *testing.T) {
	r := newRig(t)
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-1"}
	// trustScreen is already on the rig's screen, and the rig's provider declares
	// its needle — so this chat reports waiting AND has a delivery open.
	r.sweep()
	r.clock.advance(10 * termwait.DefaultDeliveryQuiet)

	r.sweep()

	assert.True(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.deliv.allSettled())
}

// A record naming some other process is a delivery this screen is no evidence
// about: the runner that received it has already been displaced by another.
func TestDetector_Sweep_OnlySettlesAgainstTheRunnerThatReceivedThePrompt(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-OTHER"}
	r.sweep()
	r.clock.advance(termwait.DefaultDeliveryQuiet)

	r.sweep()

	assert.Empty(t, r.deliv.allSettled())
}

// A journal write that fails must not latch. The record is still open, so the
// question is still live and a later tick has to ask it again.
func TestDetector_Sweep_RetriesASettleThatCouldNotBePersisted(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.deliv.settleErr = errBoom
	r.sweep()
	r.clock.advance(termwait.DefaultDeliveryQuiet)
	r.sweep()
	require.Empty(t, r.deliv.allSettled())

	r.deliv.settleErr = nil
	r.sweep()

	assert.Equal(t, []string{"req-1"}, r.deliv.allSettled())
}

// One quiet screen retires one delivery. Without the latch the journal would be
// written on every tick for as long as the CLI sat at its composer.
func TestDetector_Sweep_SettlesOnce(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.sweep()
	r.clock.advance(termwait.DefaultDeliveryQuiet)

	r.sweep()
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-1"}
	r.sweep()
	r.sweep()

	assert.Equal(t, []string{"req-1"}, r.deliv.allSettled())
}

// A chat that is Working took the delivery and turned it into a turn, which is the
// ordinary case. Its record is retired by the provider's own hook, and this gate
// must not reach for it — the stall question owns that branch.
func TestDetector_Sweep_NeverSettlesADeliveryOnAWorkingChat(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.chats.byID[chatID] = domain.AgentChat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.sweep()
	r.clock.advance(10 * termwait.DefaultDeliveryQuiet)

	r.sweep()

	assert.Empty(t, r.deliv.allSettled())
	assert.Zero(t, r.deliv.asked, "a busy chat's journal is never read")
}

// A daemon built without the port settles nothing, which is the behaviour every
// existing harness has and the safe direction to fail in.
func TestDetector_Sweep_SettlesNothingWithoutTheDeliveriesPort(t *testing.T) {
	r := newRig(t)
	r.delivering()
	waitOnly := termwait.New(termwait.Deps{
		Runners: r.runners,
		Chats:   r.chats,
		Choices: r.choices,
		Screens: r.screens,
		Prompts: r.prompts,
		Now:     r.clock.Now,
	})
	waitOnly.Sweep(context.Background(), r.rec.publish)
	r.clock.advance(10 * termwait.DefaultDeliveryQuiet)

	waitOnly.Sweep(context.Background(), r.rec.publish)

	assert.Empty(t, r.deliv.allSettled())
	assert.Zero(t, r.deliv.asked)
}

// A settle that retires NOTHING must not latch. The record is still open, so the
// question is still live — latching on the absence of an error would shut it
// against a delivery that is exactly as stuck as it was.
func TestDetector_Sweep_DoesNotLatchWhenNothingWasRetired(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.deliv.declineSettle = true
	r.sweep()
	r.clock.advance(termwait.DefaultDeliveryQuiet)
	r.sweep()
	require.Empty(t, r.deliv.allSettled())

	r.deliv.declineSettle = false
	r.sweep()

	assert.Equal(t, []string{"req-1"}, r.deliv.allSettled())
}
