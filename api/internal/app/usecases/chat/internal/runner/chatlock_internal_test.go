package runner

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// newTestLock is the ingress's own chat locks.
func newTestLock() *ingress.Locks { return ingress.NewLocks() }

func newLockedRunners() *Runners {
	return &Runners{chatLock: newTestLock(), parks: newParks(), live: newLiveTable()}
}

// held is chat-1 held by a user-initiated path.
func held(t *testing.T, rs *Runners) *hold {
	t.Helper()
	h, err := rs.acquire(t.Context(), "chat-1")
	require.NoError(t, err)
	return h
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// hookOn records a hook of chat-1 the way the ingress does, under the chat's
// lock, and closes the returned channel once it has.
func hookOn(ctx context.Context, rs *Runners) <-chan struct{} {
	recorded := make(chan struct{})
	go func() {
		if unlock, err := rs.chatLock.Lock(ctx, "chat-1"); err == nil {
			unlock()
		}
		close(recorded)
	}()
	return recorded
}

// A wait outside the lock leaves the chat to its hooks: the lock is free for
// the whole wait, and held again once it returns.
func TestHold_AWaitRunsWithTheChatLockReleased(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rs := newLockedRunners()
		h := held(t, rs)
		defer h.release()

		err := h.outside(func(ctx context.Context) error {
			<-hookOn(ctx, rs) // a hook of the chat records meanwhile
			return nil
		})

		require.NoError(t, err)
		recorded := hookOn(t.Context(), rs)
		synctest.Wait()
		assert.False(t, isClosed(recorded), "the hold took the lock back after its wait")
		h.release()
		synctest.Wait()
		assert.True(t, isClosed(recorded))
	})
}

// A stop cancels a wait parked on the chat, and the path that waited reports
// it was stopped rather than deciding again.
func TestHold_AStopAbandonsAParkedWait(t *testing.T) {
	rs := newLockedRunners()
	h := held(t, rs)
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
	release, err := rs.HoldChat(t.Context(), "chat-1")
	require.NoError(t, err)
	release()

	require.ErrorIs(t, <-done, ErrStopped)
}

// A path queued for the chat when a stop begins is turned away, not run after
// the stop on the state the stop left.
func TestAcquire_APathQueuedWhenAStopBeginsIsStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rs := newLockedRunners()
		unlockHook, err := rs.chatLock.Lock(t.Context(), "chat-1") // a hook mid-record
		require.NoError(t, err)
		acquired := make(chan error, 1)
		go func() {
			h, err := rs.acquire(context.Background(), "chat-1")
			if err == nil {
				h.release()
			}
			acquired <- err
		}()
		synctest.Wait()

		stopped := make(chan struct{})
		go func() {
			if release, err := rs.HoldChat(context.Background(), "chat-1"); err == nil {
				release()
			}
			close(stopped)
		}()

		require.ErrorIs(t, <-acquired, ErrStopped)
		unlockHook()
		<-stopped
		held(t, rs).release() // a path that starts after the stop holds the chat
	})
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

// noticeTurns closes failed once it is asked to fail a turn on a notice.
type noticeTurns struct {
	noopTurns
	failed chan struct{}
}

func (n noticeTurns) FailTurnOnNotice(context.Context, engineagents.Runner, engineagents.TerminalNotice) {
	close(n.failed)
}

// A notice is recorded under its chat's lock: a hook mid-record finishes first,
// so the notice reads the turn that hook left.
func TestFailTurnOnNotice_WaitsForTheChatsHook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rs := newLockedRunners()
		turns := noticeTurns{failed: make(chan struct{})}
		rs.turns = turns
		unlock, err := rs.chatLock.Lock(t.Context(), "chat-1") // a hook mid-record
		require.NoError(t, err)

		go rs.failTurnOnNotice(context.Background(), engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1"},
			engineagents.TerminalNotice{Kind: "usage_limit"})
		synctest.Wait()
		assert.False(t, isClosed(turns.failed), "the notice was recorded while the chat's hook held the lock")

		unlock()
		synctest.Wait()
		assert.True(t, isClosed(turns.failed))
	})
}
