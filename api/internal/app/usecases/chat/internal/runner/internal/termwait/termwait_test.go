package termwait_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var errBoom = errors.New("read failed")

type fakeRunners struct {
	mu   sync.Mutex
	live []engineagents.Runner
	err  error

	calls int
}

func (f *fakeRunners) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeRunners) AllLive(context.Context) ([]engineagents.Runner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.live, nil
}

// fakeBusy is the ledger's busy answer, the same for every chat.
type fakeBusy struct {
	mu   sync.Mutex
	busy bool
	err  error
}

func (f *fakeBusy) set(busy bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.busy = busy
}

func (f *fakeBusy) ChatBusy(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	return f.busy, nil
}

type fakeChoices struct {
	pending map[string][]domain.ActivityChoice
	err     error
	asked   int
}

func (f *fakeChoices) PendingChoices(
	_ context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	f.asked++
	if f.err != nil {
		return nil, f.err
	}
	return f.pending[chatID], nil
}

type fakeScreens struct {
	mu sync.Mutex

	text map[string]string
	gen  map[string]uint64

	renders int

	absent map[string]bool
}

func newScreens() *fakeScreens {
	return &fakeScreens{
		text:   map[string]string{},
		gen:    map[string]uint64{},
		absent: map[string]bool{},
	}
}

func (f *fakeScreens) set(sessionID, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.text[sessionID] = text
	f.gen[sessionID]++
}

func (f *fakeScreens) repaint(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gen[sessionID]++
}

func (f *fakeScreens) Screen(sessionID string, since uint64) (string, uint64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.absent[sessionID] {
		return "", 0, false
	}
	gen := f.gen[sessionID]
	if gen == 0 || gen == since {
		return "", gen, false
	}
	f.renders++
	return f.text[sessionID], gen, true
}

func (f *fakeScreens) renderCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.renders
}

type fakePrompts struct {
	needles map[string][]engineagents.TerminalPrompt
	asked   int
}

func (f *fakePrompts) MatchTerminalPrompt(
	_ context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	f.asked++
	for _, p := range f.needles[providerID] {
		if p.Needle != "" && strings.Contains(screen, p.Needle) {
			return p, true
		}
	}
	return engineagents.TerminalPrompt{}, false
}

type recorder struct {
	mu   sync.Mutex
	sent []published
}

type published struct {
	chatID      string
	workspaceID string
	wait        domain.AgentTerminalWait
}

func (r *recorder) publish(chatID, workspaceID string, wait domain.AgentTerminalWait) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, published{chatID, workspaceID, wait})
}

func (r *recorder) all() []published {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]published(nil), r.sent...)
}

type fakeNotices struct {
	needles map[string][]engineagents.TerminalNotice
	asked   int
}

func (f *fakeNotices) MatchTerminalNotice(
	_ context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	f.asked++
	for _, n := range f.needles[providerID] {
		if n.Needle != "" && strings.Contains(screen, n.Needle) {
			return n, true
		}
	}
	return engineagents.TerminalNotice{}, false
}

// ended records each notice the detector reports as ending a turn.
type ended struct {
	mu   sync.Mutex
	seen []engineagents.TerminalNotice
	on   []engineagents.Runner
}

func (e *ended) onNotice(_ context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = append(e.seen, notice)
	e.on = append(e.on, runner)
}

func (e *ended) all() []engineagents.TerminalNotice {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]engineagents.TerminalNotice(nil), e.seen...)
}

const (
	chatID  = "chat-1"
	wsID    = "ws-1"
	session = "pty-1"

	trustScreen = "❯ 1. Yes, I trust this folder\n  Enter to confirm · Esc to cancel"
	idleScreen  = "> Ready.\n  shift+tab to cycle"

	usageLimitScreen = "■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit\n" +
		"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026\n" +
		"12:30 PM."
	usageLimitNeedle = "You've hit your usage limit"

	usageLimitSentence = "■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit " +
		"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026 " +
		"12:30 PM."
)

type rig struct {
	detector termwait.Detector
	runners  *fakeRunners
	busy     *fakeBusy
	choices  *fakeChoices
	screens  *fakeScreens
	prompts  *fakePrompts
	notices  *fakeNotices
	rec      *recorder
	ended    *ended
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newRigEvery(t, 0)
}

func newRigEvery(t *testing.T, interval time.Duration) *rig {
	t.Helper()
	runners := &fakeRunners{live: []engineagents.Runner{{
		ID:              "runner-1",
		WorkspaceID:     wsID,
		ProviderID:      "claude",
		TerminalSession: session,
		CurrentChatID:   chatID,
	}}}
	screens := newScreens()
	screens.set(session, trustScreen)
	r := &rig{
		runners: runners, busy: &fakeBusy{}, screens: screens,
		choices: &fakeChoices{pending: map[string][]domain.ActivityChoice{}},
		prompts: &fakePrompts{needles: map[string][]engineagents.TerminalPrompt{
			"claude": {{Kind: domain.AgentTerminalWaitTrust, Needle: "I trust this folder"}},
		}},
		notices: &fakeNotices{needles: map[string][]engineagents.TerminalNotice{
			"codex": {{
				Kind:     engineagents.TerminalNoticeUsageLimit,
				Needle:   usageLimitNeedle,
				Text:     usageLimitSentence,
				EndsTurn: true,
			}},
		}},
		rec: &recorder{}, ended: &ended{},
	}
	r.detector = termwait.New(termwait.Deps{
		Runners:  runners,
		Busy:     r.busy,
		Choices:  r.choices,
		Screens:  screens,
		Prompts:  r.prompts,
		Notices:  r.notices,
		OnNotice: r.ended.onNotice,
		Interval: interval,
	})
	return r
}

// wedged is a busy codex chat whose screen shows the usage-limit banner.
func (r *rig) wedged() {
	r.runners.live[0].ProviderID = "codex"
	r.busy.set(true)
	r.screens.set(session, usageLimitScreen)
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

func TestDetector_Sweep_UnrecognisedPromptCarriesNoKind(t *testing.T) {
	r := newRig(t)
	r.prompts.needles["claude"] = []engineagents.TerminalPrompt{{Needle: "Enter to confirm"}}

	r.sweep()

	assert.Equal(t, domain.AgentTerminalWait{Waiting: true}, r.detector.Wait(chatID))
}

func TestDetector_Sweep_WorkingChatIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.busy.set(true)

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.choices.asked, "a busy tick must not reach the choices read")
}

func TestDetector_Sweep_PendingChoiceIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.choices.pending[chatID] = []domain.ActivityChoice{{ID: "choice-1", ChatID: chatID}}

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.prompts.asked, "gate 3 must short-circuit before the screen match")
}

func TestDetector_Sweep_ProviderDeclaringNoNeedlesNeverWaits(t *testing.T) {
	r := newRig(t)
	r.prompts.needles = map[string][]engineagents.TerminalPrompt{}

	r.sweep()

	assert.Equal(t, domain.AgentTerminalWait{}, r.detector.Wait(chatID))
	assert.Empty(t, r.rec.all())
}

func TestDetector_Sweep_ChatWithNoLiveRunnerIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.runners.live = nil

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
}

func TestDetector_Sweep_RunnerWithNoPTYIsNeverWaiting(t *testing.T) {
	r := newRig(t)
	r.runners.live[0].TerminalSession = ""

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Zero(t, r.prompts.asked)
}

func TestDetector_Sweep_DisplacedRunnerNamesNoChat(t *testing.T) {
	r := newRig(t)
	r.runners.live[0].CurrentChatID = ""

	r.sweep()

	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.prompts.asked)
}

func TestDetector_Sweep_PublishesOnlyOnChange(t *testing.T) {
	r := newRig(t)

	r.sweep()
	r.sweep()
	r.sweep()

	assert.Len(t, r.rec.all(), 1)
}

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

func TestDetector_Sweep_DepartedChatThatWasNotWaitingIsSilent(t *testing.T) {
	r := newRig(t)
	r.screens.set(session, idleScreen)
	r.sweep()

	r.runners.live = nil
	r.sweep()

	assert.Empty(t, r.rec.all())
}

func TestDetector_Sweep_ReplacedRunnerRereadsTheScreen(t *testing.T) {
	r := newRig(t)
	r.sweep()
	require.True(t, r.detector.Wait(chatID).Waiting)

	r.runners.live[0].TerminalSession = "pty-2"
	r.screens.set("pty-2", idleScreen)
	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
}

func TestDetector_Sweep_SessionThatStopsAnsweringDropsTheVerdict(t *testing.T) {
	r := newRig(t)
	r.sweep()
	require.True(t, r.detector.Wait(chatID).Waiting)

	r.screens.absent[session] = true
	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
}

func TestDetector_Sweep_ScreenCacheSurvivesABusyTurn(t *testing.T) {
	r := newRig(t)
	r.sweep()
	renders := r.screens.renderCount()

	r.busy.set(true)
	r.sweep()
	r.busy.set(false)
	r.sweep()

	assert.Equal(t, renders, r.screens.renderCount())
	assert.True(t, r.detector.Wait(chatID).Waiting)
}

func TestDetector_Sweep_CensusFailureChangesNothing(t *testing.T) {
	r := newRig(t)
	r.sweep()

	r.runners.err = errBoom
	r.sweep()

	assert.True(t, r.detector.Wait(chatID).Waiting)
	assert.Len(t, r.rec.all(), 1)
}

func TestDetector_Sweep_BusyReadFailureIsSilent(t *testing.T) {
	r := newRig(t)
	r.busy.err = errBoom

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

func TestDetector_Sweep_NilPublisherIsSafe(t *testing.T) {
	r := newRig(t)

	r.detector.Sweep(context.Background(), nil)

	assert.True(t, r.detector.Wait(chatID).Waiting)
}

func TestDetector_Wait_UnknownChatIsNotWaiting(t *testing.T) {
	r := newRig(t)

	assert.Equal(t, domain.AgentTerminalWait{}, r.detector.Wait("never-heard-of-it"))
}

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

	r.screens.set(session, idleScreen)
	select {
	case wait := <-got:
		assert.False(t, wait.Waiting)
	case <-time.After(5 * time.Second):
		t.Fatal("the cadence stopped after its first pass")
	}
}

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

	assert.Equal(t, 1, r.runners.callCount(), "the loop must not tick again after its context is done")
}

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

// A notice that ends the turn fails it the moment it appears on a busy chat's
// screen: the screen is the only place the provider says so.
func TestDetector_Sweep_ReportsANoticeThatEndsTheTurn(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()

	got := r.ended.all()
	require.Len(t, got, 1)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, got[0].Kind)
	assert.Equal(t, "runner-1", r.ended.on[0].ID)
	assert.Empty(t, r.rec.all(), "a busy chat is never waiting")
}

func TestDetector_Sweep_ANoticeIsReportedOnceWhileItStays(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()
	r.screens.set(session, usageLimitScreen+"\n  more output")
	r.sweep()
	r.screens.repaint(session)
	r.sweep()

	assert.Len(t, r.ended.all(), 1)
}

func TestDetector_Sweep_ANoticeOnAReplacedRunnersScreenIsReportedAgain(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.runners.live[0].TerminalSession = "pty-2"
	r.screens.set("pty-2", usageLimitScreen)
	r.sweep()

	assert.Len(t, r.ended.all(), 2)
}

func TestDetector_Sweep_ANoticeReturningAfterItScrolledAwayIsReportedAgain(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.screens.set(session, idleScreen)
	r.sweep()
	r.screens.set(session, usageLimitScreen)
	r.sweep()

	assert.Len(t, r.ended.all(), 2)
}

func TestDetector_Sweep_ANoticeNeverEndsAnIdleChatsTurn(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.busy.set(false)

	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_ABannerLeftFromAnEarlierTurnEndsNothing(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.busy.set(false)
	r.sweep()

	r.busy.set(true)
	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_NoticeWithoutEndsTurnEndsNothing(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.notices.needles["codex"] = []engineagents.TerminalNotice{{
		Kind:   engineagents.TerminalNoticeUsageLimit,
		Needle: usageLimitNeedle,
		Text:   usageLimitSentence,
	}}

	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_ProviderDeclaringNoNoticesEndsNothing(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.runners.live[0].ProviderID = "claude"

	r.sweep()

	assert.Empty(t, r.ended.all())
}

func TestDetector_Sweep_WithoutNoticeDependenciesNothingEnds(t *testing.T) {
	r := newRig(t)
	r.wedged()
	waitOnly := termwait.New(termwait.Deps{
		Runners: r.runners,
		Busy:    r.busy,
		Choices: r.choices,
		Screens: r.screens,
		Prompts: r.prompts,
	})

	waitOnly.Sweep(context.Background(), r.rec.publish)

	assert.Empty(t, r.ended.all())
	assert.Zero(t, r.notices.asked)
}

// §6a: an idle daemon does no periodic work. With no live runner the sweep
// parks after one pass — no ticker, no read transaction every interval — and a
// runner being recorded live wakes it.
func TestDetector_Run_ParksWhileNoRunnerIsLiveAndWakesOnDemand(t *testing.T) {
	r := newRigEvery(t, time.Millisecond)
	r.runners.mu.Lock()
	live := r.runners.live
	r.runners.live = nil
	r.runners.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.detector.Run(ctx, nil)
	require.Eventually(t, func() bool { return r.runners.callCount() == 1 }, 5*time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, r.runners.callCount(), "nothing to sweep: the loop must park, not tick")

	r.runners.mu.Lock()
	r.runners.live = live
	r.runners.mu.Unlock()
	r.detector.Wake()
	require.Eventually(t, func() bool { return r.runners.callCount() > 3 }, 5*time.Second, time.Millisecond,
		"a live runner resumes the cadence")
}

// The idle report arrives over the api channel, and an api-channel runner has
