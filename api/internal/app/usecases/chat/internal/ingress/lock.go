package ingress

import (
	"context"
	"fmt"
	"sync"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

type Locks struct {
	mu    sync.Mutex
	chats map[string]*chatLock
}

type chatLock struct {
	held chan struct{}
	refs int
}

func NewLocks() *Locks {
	return &Locks{chats: map[string]*chatLock{}}
}

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

type Locker interface {
	Lock(ctx context.Context, chatID string) (unlock func(), err error)
}

type Writer struct {
	ledger *ledgerstore.Store
	locks  Locker
}

func NewWriter(store *ledgerstore.Store, locks Locker) Writer {
	return Writer{ledger: store, locks: locks}
}

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
