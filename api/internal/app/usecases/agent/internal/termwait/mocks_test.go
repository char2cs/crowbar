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
