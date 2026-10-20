package ingress

import "sync"

// Locks is one mutex per chat. Every hook of a chat holds it while it records,
// and every user-initiated spawn on the chat holds it for its decision and its
// runner_started row. A chat nobody holds keeps no entry.
type Locks struct {
	mu    sync.Mutex
	chats map[string]*chatLock
}

type chatLock struct {
	mu   sync.Mutex
	refs int
}

// NewLocks returns a set of chat locks nobody holds.
func NewLocks() *Locks {
	return &Locks{chats: map[string]*chatLock{}}
}

// Lock blocks until chatID is free and returns its release. Releasing twice is
// a no-op, so a deferred release can follow an early one.
func (l *Locks) Lock(chatID string) (unlock func()) {
	l.mu.Lock()
	c, ok := l.chats[chatID]
	if !ok {
		c = &chatLock{}
		l.chats[chatID] = c
	}
	c.refs++
	l.mu.Unlock()

	c.mu.Lock()
	return sync.OnceFunc(func() {
		c.mu.Unlock()
		l.mu.Lock()
		defer l.mu.Unlock()
		if c.refs--; c.refs == 0 {
			delete(l.chats, chatID)
		}
	})
}
