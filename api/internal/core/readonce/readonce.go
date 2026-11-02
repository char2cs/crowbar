// Package readonce lets one long read share the store reads it repeats: the
// sidebar's snapshot reads each project's tree, every chat and every
// workspace once instead of once per repo.
package readonce

import (
	"context"
	"sync"
)

type sessionKey struct{}

// session holds one read's answers until it ends; after that every read is fresh,
// so work the read started in the background never sees them.
type session struct {
	mu    sync.Mutex
	ended bool
	vals  map[string]*answer
}

type answer struct {
	once sync.Once
	v    any
	err  error
}

// Begin starts a read whose repeated reads through Do are answered once; end
// releases them.
func Begin(ctx context.Context) (context.Context, func()) {
	s := &session{vals: map[string]*answer{}}
	return context.WithValue(ctx, sessionKey{}, s), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ended, s.vals = true, nil
	}
}

// Do answers key once per read begun on ctx, through read; outside one it just reads.
// A value shared this way must not be written to by its readers.
func Do[T any](ctx context.Context, key string, read func() (T, error)) (T, error) {
	s, _ := ctx.Value(sessionKey{}).(*session)
	if s == nil {
		return read()
	}
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return read()
	}
	a := s.vals[key]
	if a == nil {
		a = &answer{}
		s.vals[key] = a
	}
	s.mu.Unlock()
	a.once.Do(func() { a.v, a.err = read() })
	v, _ := a.v.(T)
	return v, a.err
}
