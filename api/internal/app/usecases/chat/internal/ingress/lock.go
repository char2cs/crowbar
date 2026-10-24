package ingress

import (
	"context"
	"fmt"
	"sync"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Locks is one lock per chat, taken before the ledger's own writer by every
// hook, every user-initiated path and every other write (Writer) of the chat.
// A chat nobody holds or waits on keeps no entry.
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

// Locker takes a chat's lock.
type Locker interface {
	Lock(ctx context.Context, chatID string) (unlock func(), err error)
}

// Writer writes a chat's ledger for a path that is not a hook: the chat's
// lock first, then the ledger's writer, the order every hook takes them in.
type Writer struct {
	ledger *ledgerstore.Store
	locks  Locker
}

// NewWriter is a Writer of store under locks.
func NewWriter(store *ledgerstore.Store, locks Locker) Writer {
	return Writer{ledger: store, locks: locks}
}

// Do commits the rows fn decides on chatID's ledger, and returns them.
func (w Writer) Do(ctx context.Context, chatID string, fn func(ledger.State) ([]ledger.Row, error)) ([]ledger.Row, error) {
	unlock, err := w.locks.Lock(ctx, chatID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, _, err := w.ledger.Do(ctx, chatID, fn)
	if err != nil {
		return nil, fmt.Errorf("ingress: write chat %s: %w", chatID, err)
	}
	return rows, nil
}
