package termwait_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var errBoom = errors.New("read failed")

type fakeRunners struct {
	live []engineagents.Runner
	err  error

	calls int
}

func (f *fakeRunners) AllLive(context.Context) ([]engineagents.Runner, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.live, nil
}

type fakeChats struct {
	byID map[string]domain.Chat
	err  error
}

func (f *fakeChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	if f.err != nil {
		return domain.Chat{}, f.err
	}
	return f.byID[id], nil
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

type fakeWork struct {
	open  bool
	err   error
	asked int
}

func (f *fakeWork) OpenWork(context.Context, string) (bool, error) {
	f.asked++
	if f.err != nil {
		return false, f.err
	}
	return f.open, nil
}

type stalls struct {
	mu   sync.Mutex
	seen []seam.Stall
}

func (s *stalls) onStall(_ context.Context, stall seam.Stall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, stall)
}

func (s *stalls) all() []seam.Stall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]seam.Stall(nil), s.seen...)
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 8, 18, 16, 26, 34, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeDeliveries struct {
	mu        sync.Mutex
	pending   map[string]termwait.Delivery
	err       error
	asked     int
	settled   []string
	settleErr error

	declineSettle bool
}

func (f *fakeDeliveries) PendingDelivery(
	_ context.Context,
	chatID string,
) (termwait.Delivery, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	if f.err != nil {
		return termwait.Delivery{}, false
	}
	delivery, ok := f.pending[chatID]
	return delivery, ok
}

func (f *fakeDeliveries) SettleDelivery(_ context.Context, chatID, requestID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.settleErr != nil {
		return false, f.settleErr
	}
	if f.declineSettle {
		return false, nil
	}
	f.settled = append(f.settled, requestID)
	delete(f.pending, chatID)
	return true, nil
}

func (f *fakeDeliveries) allSettled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.settled...)
}

type fakeMessages struct {
	mu         sync.Mutex
	since      time.Time
	unfinished bool
	abandoned  int
	closed     bool
	err        error
	asked      int
}

func (f *fakeMessages) UnfinishedSince(string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked++
	return f.since, f.unfinished
}

func (f *fakeMessages) AbandonMessage(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	f.abandoned++
	f.unfinished = false
	return f.closed, nil
}

func (f *fakeMessages) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.abandoned
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
	chats    *fakeChats
	choices  *fakeChoices
	screens  *fakeScreens
	prompts  *fakePrompts
	notices  *fakeNotices
	work     *fakeWork
	deliv    *fakeDeliveries
	msgs     *fakeMessages
	clock    *clock
	rec      *recorder
	stalls   *stalls
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
	chats := &fakeChats{byID: map[string]domain.Chat{
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
		msgs:  &fakeMessages{closed: true},
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
		Messages:   r.msgs,
		Interval:   interval,
		Now:        r.clock.Now,
	})
	return r
}

func (r *rig) wedged() {
	r.runners.live[0].ProviderID = "codex"
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.screens.set(session, usageLimitScreen)
}

func (r *rig) delivering() {
	r.screens.set(session, idleScreen)
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-1"}
}

func (r *rig) cutOff() {
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.msgs.unfinished = true
	r.msgs.since = r.clock.Now()
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
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID, Working: true}

	r.sweep()

	assert.False(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.rec.all())
	assert.Zero(t, r.choices.asked, "a busy tick must not reach the choices read")
	assert.Zero(t, r.work.asked, "a busy tick must not reach the open-work read")
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

	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.sweep()
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID}
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

	assert.Equal(t, 1, r.runners.calls, "the loop must not tick again after its context is done")
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

func TestRegression_StalledTurnIsClosedWhenTheScreenHasBeenQuiet(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()
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

func TestRegression_StalledTurnIsNotClosedBeforeTheQuietPeriod(t *testing.T) {
	r := newRig(t)
	r.wedged()

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet - time.Second)
	r.sweep()

	assert.Empty(t, r.stalls.all())
	assert.Zero(t, r.work.asked, "the repository gates must not be reached before the clock elapses")
}

func TestDetector_Sweep_MovingScreenIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	previous := usageLimitScreen

	for i := 0; i < 50; i++ {
		r.clock.advance(termwait.DefaultStallQuiet)

		frame := usageLimitScreen + "\n  working (" + strconv.Itoa(i) + "s)"
		require.NotEqual(t, previous, frame, "the fixture must move the TEXT, not just the generation")
		previous = frame
		r.screens.set(session, frame)
		r.sweep()

		require.Empty(t, r.stalls.all(), "a moving screen must never close a turn")
		require.True(t, r.chats.byID[chatID].Working, "and the chat must still be working")
	}
}

func TestDetector_Sweep_ProviderDeclaringNoNoticesNeverCloses(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.runners.live[0].ProviderID = "claude"

	r.sweep()
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

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

func TestDetector_Sweep_IdleChatIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID}

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

func TestDetector_Sweep_PendingChoiceIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.choices.pending[chatID] = []domain.ActivityChoice{{ID: "choice-1", ChatID: chatID}}

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

func TestDetector_Sweep_OpenToolCallIsNeverClosed(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.work.open = true

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	require.Empty(t, r.stalls.all(), "a chat mid-tool is working, whatever its screen shows")

	r.work.open = false
	r.sweep()

	assert.Len(t, r.stalls.all(), 1)
}

func TestDetector_Sweep_OpenWorkReadFailureIsSilent(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.work.err = errBoom

	r.sweep()
	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

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

func TestDetector_Sweep_BannerScrollingAwayWithdrawsTheEvidence(t *testing.T) {
	r := newRig(t)
	r.wedged()
	r.sweep()

	r.screens.set(session, idleScreen)
	r.clock.advance(100 * termwait.DefaultStallQuiet)
	r.sweep()

	assert.Empty(t, r.stalls.all())
}

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

	r.clock.advance(termwait.DefaultStallQuiet)
	r.sweep()
	assert.Len(t, r.stalls.all(), 1)
}

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

func TestDetector_Sweep_SettlesADeliveryThatProducedNoTurn(t *testing.T) {
	r := newRig(t)
	r.delivering()

	r.sweep()
	assert.Empty(t, r.deliv.allSettled(), "the quiet clock has not started yet")

	r.clock.advance(termwait.DefaultDeliveryQuiet)
	r.sweep()

	assert.Equal(t, []string{"req-1"}, r.deliv.allSettled())
}

func TestDetector_Sweep_LeavesADeliveryAloneUntilTheScreenHasBeenStillLongEnough(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.sweep()

	r.clock.advance(termwait.DefaultDeliveryQuiet - time.Second)
	r.sweep()

	assert.Empty(t, r.deliv.allSettled())
}

func TestDetector_Sweep_NeverSettlesADeliveryBehindABlockingModal(t *testing.T) {
	r := newRig(t)
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-1"}

	r.sweep()
	r.clock.advance(10 * termwait.DefaultDeliveryQuiet)

	r.sweep()

	assert.True(t, r.detector.Wait(chatID).Waiting)
	assert.Empty(t, r.deliv.allSettled())
}

func TestDetector_Sweep_OnlySettlesAgainstTheRunnerThatReceivedThePrompt(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.deliv.pending[chatID] = termwait.Delivery{RequestID: "req-1", RunnerID: "runner-OTHER"}
	r.sweep()
	r.clock.advance(termwait.DefaultDeliveryQuiet)

	r.sweep()

	assert.Empty(t, r.deliv.allSettled())
}

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

func TestDetector_Sweep_NeverSettlesADeliveryOnAWorkingChat(t *testing.T) {
	r := newRig(t)
	r.delivering()
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.sweep()
	r.clock.advance(10 * termwait.DefaultDeliveryQuiet)

	r.sweep()

	assert.Empty(t, r.deliv.allSettled())
	assert.Zero(t, r.deliv.asked, "a busy chat's journal is never read")
}

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

func TestDetector_Sweep_ClosesATurnWhoseMessageWasCutOff(t *testing.T) {
	r := newRig(t)
	r.cutOff()

	r.sweep()
	assert.Zero(t, r.msgs.count(), "the quiet window has not elapsed yet")

	r.clock.advance(termwait.DefaultMessageQuiet)
	r.sweep()

	assert.Equal(t, 1, r.msgs.count())
}

func TestDetector_Sweep_LeavesAThinkingPauseAlone(t *testing.T) {
	r := newRig(t)
	r.cutOff()
	r.sweep()

	r.clock.advance(3 * time.Second)
	r.sweep()

	assert.Zero(t, r.msgs.count())
}

func TestDetector_Sweep_AGrowingMessageNeverLooksAbandoned(t *testing.T) {
	r := newRig(t)
	r.cutOff()

	for range 6 {
		r.clock.advance(termwait.DefaultMessageQuiet - time.Second)
		r.msgs.since = r.clock.Now()
		r.sweep()
	}

	assert.Zero(t, r.msgs.count())
}

func TestDetector_Sweep_NeverAbandonsAMessageWhileAToolIsOpen(t *testing.T) {
	r := newRig(t)
	r.cutOff()
	r.work.open = true
	r.sweep()
	r.clock.advance(10 * termwait.DefaultMessageQuiet)

	r.sweep()

	assert.Zero(t, r.msgs.count())
}

func TestDetector_Sweep_NeverAbandonsAMessageWithAPendingChoice(t *testing.T) {
	r := newRig(t)
	r.cutOff()
	r.choices.pending[chatID] = []domain.ActivityChoice{{ID: "choice-1"}}
	r.sweep()
	r.clock.advance(10 * termwait.DefaultMessageQuiet)

	r.sweep()

	assert.Zero(t, r.msgs.count())
}

func TestDetector_Sweep_AFinishedMessageIsNotEvidence(t *testing.T) {
	r := newRig(t)
	r.chats.byID[chatID] = domain.Chat{ID: chatID, WorkspaceID: wsID, Working: true}
	r.msgs.unfinished = false
	r.clock.advance(10 * termwait.DefaultMessageQuiet)

	r.sweep()

	assert.Zero(t, r.msgs.count())
}

func TestDetector_Sweep_AbandonsNothingWithoutTheMessagesPort(t *testing.T) {
	r := newRig(t)
	r.cutOff()
	noMessages := termwait.New(termwait.Deps{
		Runners: r.runners, Chats: r.chats, Choices: r.choices,
		Screens: r.screens, Prompts: r.prompts, Now: r.clock.Now,
	})
	r.clock.advance(10 * termwait.DefaultMessageQuiet)

	noMessages.Sweep(context.Background(), r.rec.publish)

	assert.Zero(t, r.msgs.count())
}
