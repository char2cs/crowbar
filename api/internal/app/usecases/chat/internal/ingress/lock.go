package ingress

import (
	"context"
	"fmt"
	"sync"
)

// Locks is one lock per chat. Every hook of a chat holds it while it records,
// and every user-initiated spawn on the chat holds it for its decision and its
// runner_started row. A chat nobody holds or waits on keeps no entry.
type Locks struct {
	mu    sync.Mutex
	chats map[string]*chatLock
}

type chatLock struct {
	// held is full while the chat is held: a channel, so a waiter can also
	// watch its context.
	held chan struct{}
	refs int
}

// NewLocks returns a set of chat locks nobody holds.
func NewLocks() *Locks {
	return &Locks{chats: map[string]*chatLock{}}
}

// Lock waits until chatID is free and returns its release, or gives up when
// ctx ends. Releasing twice is a no-op, so a deferred release can follow an
// early one.
func (l *Locks) Lock(ctx context.Context, chatID string) (unlock func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ingress: lock chat %s: %w", chatID, err)
	}
	l.mu.Lock()
	c, ok := l.chats[chatID]
	if !ok {
		c = &chatLock{held: make(chan struct{}, 1)}
		l.chats[chatID] = c
	}
	c.refs++
	l.mu.Unlock()

	select {
	case c.held <- struct{}{}:
	case <-ctx.Done():
		l.drop(chatID, c)
		return nil, fmt.Errorf("ingress: lock chat %s: %w", chatID, ctx.Err())
	}
	return sync.OnceFunc(func() {
		<-c.held
		l.drop(chatID, c)
	}), nil
}

func (l *Locks) drop(chatID string, c *chatLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c.refs--; c.refs == 0 {
		delete(l.chats, chatID)
	}
}
