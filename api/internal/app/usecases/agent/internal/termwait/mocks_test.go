package termwait_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Hand-written mocks, one per port. They record what was asked as well as what
// they answered, because half of what this package promises is about what it does
// NOT do: a gate that short-circuits has to be observable as a call that never
// happened.

var errBoom = errors.New("read failed")

type fakeRunners struct {
	live []domain.AgentRunner
	err  error
	// calls counts sweeps, so a test can prove the loop actually ran.
	calls int
}

func (f *fakeRunners) AllLive(context.Context) ([]domain.AgentRunner, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.live, nil
}

type fakeChats struct {
	byID map[string]domain.AgentChat
	err  error
}

func (f *fakeChats) GetChat(_ context.Context, id string) (domain.AgentChat, error) {
	if f.err != nil {
		return domain.AgentChat{}, f.err
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

// fakeScreens models the engine's (text, gen, changed) contract faithfully,
// including the "unchanged screens return no text" half — which is the whole
// reason the detector can poll without cost, so a mock that always returned text
// would hide the bug this design exists to prevent.
type fakeScreens struct {
	mu sync.Mutex
	// text and gen are the current screen and its generation, per session id.
	text map[string]string
	gen  map[string]uint64
	// renders counts reads that actually produced text — the expensive half.
	renders int
	// absent names sessions the engine cannot answer for at all.
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

// repaint models the case the generation counter cannot distinguish from a real
// change: the CLI consumed a chunk and redrew BYTE-IDENTICAL cells. The
// generation advances, so the detector must render — and the rendered text is
// the same string it already had.
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

// fakePrompts stands in for the descriptor lookup: a provider's declared needles,
// matched against a screen. Keyed by provider so "a provider that declares
// nothing" is expressible as an absent key.
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

// recorder collects published verdicts in order.
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

// fakeNotices is the second provider seam: the messages a CLI paints INSTEAD of
// finishing a turn. Keyed by provider, so "a provider that declares nothing" is
// an absent key — the degradation case this feature's safety rests on.
//
// It matches on a plain substring rather than reproducing the engine's
// whitespace-insensitive reduction, because what is under test here is the
// DETECTOR's gate ordering and clock. The reduction, the wrap-tolerant match and
// the sentence capture are tested against the real descriptors in the termprompt
// package and in the usecase's own seam test.
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

// fakeWork is the hook-evidence gate: what the conversation record still shows
// running. It counts reads because half of what the gate ordering promises is
// that this one is not reached on an ordinary tick.
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

// stalls collects the turns the detector asked to have closed.
type stalls struct {
	mu   sync.Mutex
	seen []termwait.Stall
}

func (s *stalls) onStall(_ context.Context, stall termwait.Stall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, stall)
}

func (s *stalls) all() []termwait.Stall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]termwait.Stall(nil), s.seen...)
}

// clock is the injectable time the quiet period is measured on.
//
// Every timing assertion in this package is made by ADVANCING THIS and calling
// Sweep again. Nothing sleeps and nothing waits on a real duration, so a test
// that proves a 120-second rule runs in microseconds and cannot be flaky — which
// is the only way a rule that long can be tested at all.
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

// fakeDeliveries is the prompt journal as the third question sees it. It records
// what was settled AND how often it was asked, because most of what that gate
// promises is about the ticks on which it does nothing.
type fakeDeliveries struct {
	mu        sync.Mutex
	pending   map[string]termwait.Delivery
	err       error
	asked     int
	settled   []string
	settleErr error
	// declineSettle models the journal answering "nothing to retire" — a record
	// that never reached a process, or one a hook already accounted for.
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

// fakeMessages is the assistant-message stream as the abandoned-message question
// sees it: is there an unfinished message, and when did it last grow?
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
