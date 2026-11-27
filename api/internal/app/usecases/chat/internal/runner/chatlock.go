package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type ChatLock interface {
	Lock(ctx context.Context, chatID string) (unlock func(), err error)
}

var errPreempted = errors.New("agent: preempted by a stop")

type parks struct {
	mu     sync.Mutex
	byChat map[string]*chatParks
}

type chatParks struct {
	cancels  map[*context.CancelCauseFunc]struct{}
	stopping int
	stopped  chan struct{}
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

func (p *parks) open(ctx context.Context, chatID string) (context.Context, func()) {
	park, cancel := context.WithCancelCause(ctx)
	p.mu.Lock()
	e := p.entry(chatID)
	e.cancels[&cancel] = struct{}{}
	stopped, waits := e.stopped, e.stopping > 0
	p.mu.Unlock()
	if waits {
		select {
		case <-stopped:
		case <-park.Done():
		}
	}
	return park, func() {
		cancel(nil)
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(e.cancels, &cancel)
		p.dropIfIdle(chatID, e)
	}
}

func (p *parks) preempt(chatID string) (release func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entry(chatID)
	if e.stopping == 0 {
		e.stopped = make(chan struct{})
	}
	e.stopping++
	for cancel := range e.cancels {
		(*cancel)(errPreempted)
	}
	return sync.OnceFunc(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if e.stopping--; e.stopping == 0 {
			close(e.stopped)
		}
		p.dropIfIdle(chatID, e)
	})
}

func parkErr(park context.Context, err error) error {
	if errors.Is(context.Cause(park), errPreempted) {
		return ErrStopped
	}
	return err
}

type hold struct {
	lock   ChatLock
	chatID string
	park   context.Context
	unlock func()
	close  func()
}

func (rs *Runners) acquire(ctx context.Context, chatID string) (*hold, error) {
	park, closePark := rs.parks.open(ctx, chatID)
	unlock, err := rs.chatLock.Lock(park, chatID)
	if err != nil {
		closePark()
		return nil, parkErr(park, err)
	}
	return &hold{lock: rs.chatLock, chatID: chatID, park: park, unlock: unlock, close: closePark}, nil
}

func (h *hold) release() {
	h.unlock()
	h.close()
}

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
