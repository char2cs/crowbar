package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// testLock is one mutex per chat, as the ingress's chat locks are.
type testLock struct {
	mu    sync.Mutex
	chats map[string]*sync.Mutex
}

func newTestLock() *testLock { return &testLock{chats: map[string]*sync.Mutex{}} }

func (l *testLock) Lock(chatID string) func() {
	l.mu.Lock()
	m, ok := l.chats[chatID]
	if !ok {
		m = &sync.Mutex{}
		l.chats[chatID] = m
	}
	l.mu.Unlock()
	m.Lock()
	return sync.OnceFunc(m.Unlock)
}

func newLockedRunners() *Runners {
	return &Runners{chatLock: newTestLock(), parks: newParks(), live: newLiveTable()}
}

// A wait outside the lock leaves the chat to its hooks: the lock is free for
// the whole wait, and held again once it returns.
func TestHold_AWaitRunsWithTheChatLockReleased(t *testing.T) {
	rs := newLockedRunners()
	h := rs.acquire(t.Context(), "chat-1")
	defer h.release()

	err := h.outside(func(context.Context) error {
		rs.chatLock.Lock("chat-1")() // a hook of the chat records meanwhile
		return nil
	})

	require.NoError(t, err)
	var released atomic.Bool
	takenAfterRelease := make(chan bool)
	go func() {
		rs.chatLock.Lock("chat-1")()
		takenAfterRelease <- released.Load()
	}()
	released.Store(true)
	h.release()
	assert.True(t, <-takenAfterRelease, "the hold took the lock back after its wait")
}

// A stop cancels a wait parked on the chat, and the path that waited reports
// it was stopped rather than deciding again.
func TestHold_AStopAbandonsAParkedWait(t *testing.T) {
	rs := newLockedRunners()
	h := rs.acquire(t.Context(), "chat-1")
	defer h.release()
	parked := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- h.outside(func(park context.Context) error {
			close(parked)
			<-park.Done()
			return park.Err()
		})
	}()
	<-parked
	release := rs.HoldChat("chat-1")
	release()

	require.ErrorIs(t, <-done, ErrStopped)
}

// A wait that begins while a stop is under way starts cancelled: the stop is
// never queued behind it.
func TestParks_AParkOpenedDuringAStopStartsCancelled(t *testing.T) {
	p := newParks()
	stopping := p.preempt("chat-1")

	park, closePark := p.open(t.Context(), "chat-1")
	defer closePark()

	require.ErrorIs(t, parkErr(park, park.Err()), ErrStopped)
	stopping()
	other, closeOther := p.open(t.Context(), "chat-1")
	defer closeOther()
	assert.NoError(t, other.Err(), "a park opened after the stop is not cancelled")
}

// noticeTurns reports, for each notice it is asked to fail a turn on, whether
// the chat's hook had released its lock by then.
type noticeTurns struct {
	noopTurns
	released *atomic.Bool
	failed   chan bool
}

func (n noticeTurns) FailTurnOnNotice(context.Context, engineagents.Runner, engineagents.TerminalNotice) {
	n.failed <- n.released.Load()
}

// A notice is recorded under its chat's lock: a hook mid-record finishes first,
// so the notice reads the turn that hook left.
func TestFailTurnOnNotice_WaitsForTheChatsHook(t *testing.T) {
	rs := newLockedRunners()
	turns := noticeTurns{released: &atomic.Bool{}, failed: make(chan bool)}
	rs.turns = turns
	unlock := rs.chatLock.Lock("chat-1")

	go rs.failTurnOnNotice(t.Context(), engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"},
		engineagents.TerminalNotice{Kind: "usage_limit"})
	turns.released.Store(true)
	unlock()

	assert.True(t, <-turns.failed, "the notice was recorded while the chat's hook still held the lock")
}
