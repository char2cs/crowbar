// Package runner (file chatlock.go) is how a user-initiated path holds a chat:
// by the chat's lock, the one every hook of the chat is recorded under. A path
// holds it while it decides and while it records runner_started, and releases
// it around every wait, because what it waits for (a CLI's exit, a prompt's
// acknowledgement) needs that CLI's own hooks recorded.
package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ChatLock serialises one chat's hooks and its user-initiated paths. A waiter
// gives up when its ctx ends.
type ChatLock interface {
	Lock(ctx context.Context, chatID string) (unlock func(), err error)
}

// errPreempted is the cause a parked wait is cancelled with by a stop.
var errPreempted = errors.New("agent: preempted by a stop")

// parks are the waits user-initiated paths sit in outside their chat's lock. A
// stop cancels them, so it is never queued behind a wait on the turn it ends.
type parks struct {
	mu     sync.Mutex
	byChat map[string]*chatParks
}

type chatParks struct {
	cancels map[*context.CancelCauseFunc]struct{}
	// stopping counts stops under way: a park opened meanwhile starts cancelled.
	stopping int
}

func newParks() *parks { return &parks{byChat: map[string]*chatParks{}} }

func (p *parks) entry(chatID string) *chatParks {
	e, ok := p.byChat[chatID]
	if !ok {
		e = &chatParks{cancels: map[*context.CancelCauseFunc]struct{}{}}
		p.byChat[chatID] = e
	}
	return e
}

func (p *parks) dropIfIdle(chatID string, e *chatParks) {
	if len(e.cancels) == 0 && e.stopping == 0 {
		delete(p.byChat, chatID)
	}
}

// open returns a context derived from ctx that a stop on chatID cancels, and
// its close.
func (p *parks) open(ctx context.Context, chatID string) (context.Context, func()) {
	park, cancel := context.WithCancelCause(ctx)
	p.mu.Lock()
	e := p.entry(chatID)
	e.cancels[&cancel] = struct{}{}
	if e.stopping > 0 {
		cancel(errPreempted)
	}
	p.mu.Unlock()
	return park, func() {
		cancel(nil)
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(e.cancels, &cancel)
		p.dropIfIdle(chatID, e)
	}
}

// preempt cancels every wait parked on chatID, and every one opened before
// the returned release.
func (p *parks) preempt(chatID string) (release func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entry(chatID)
	e.stopping++
	for cancel := range e.cancels {
		(*cancel)(errPreempted)
	}
	return sync.OnceFunc(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		e.stopping--
		p.dropIfIdle(chatID, e)
	})
}

// parkErr reports a wait a stop abandoned as ErrStopped, and passes every
// other failure through.
func parkErr(park context.Context, err error) error {
	if errors.Is(context.Cause(park), errPreempted) {
		return ErrStopped
	}
	return err
}

// hold is one user-initiated path's claim on a chat: its lock, and a park a
// stop cancels.
type hold struct {
	lock   ChatLock
	chatID string
	park   context.Context
	unlock func()
	close  func()
}

// acquire opens a park on chatID and takes the chat's lock on it: a stop under
// way, or one that begins while this path queues, turns it away (ErrStopped).
func (rs *Runners) acquire(ctx context.Context, chatID string) (*hold, error) {
	park, closePark := rs.parks.open(ctx, chatID)
	unlock, err := rs.chatLock.Lock(park, chatID)
	if err != nil {
		closePark()
		return nil, parkErr(park, err)
	}
	return &hold{lock: rs.chatLock, chatID: chatID, park: park, unlock: unlock, close: closePark}, nil
}

// release lets go of the chat.
func (h *hold) release() {
	h.unlock()
	h.close()
}

// outside runs wait on the park with the chat's lock released, and takes the
// lock back after. Whatever the caller decided before is stale: it decides
// again from what it reads now. On an error the hold may no longer hold the
// lock, and the caller returns.
func (h *hold) outside(wait func(park context.Context) error) error {
	h.unlock()
	h.unlock = func() {}
	err := wait(h.park)
	if err == nil {
		h.unlock, err = h.lock.Lock(h.park, h.chatID)
	}
	if err != nil {
		h.unlock = func() {}
		return parkErr(h.park, err)
	}
	return nil
}

// HoldChat preempts chatID's parked waits and takes its lock, for a path that
// must not interleave with a spawn and never waits under it.
func (rs *Runners) HoldChat(ctx context.Context, chatID string) (release func(), err error) {
	stopping := rs.parks.preempt(chatID)
	unlock, err := rs.chatLock.Lock(ctx, chatID)
	if err != nil {
		stopping()
		return nil, fmt.Errorf("agent: hold chat: %w", err)
	}
	return func() {
		unlock()
		stopping()
	}, nil
}
