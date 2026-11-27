package readonce

import (
	"context"
	"sync"
)

type sessionKey struct{}

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

func Begin(ctx context.Context) (context.Context, func()) {
	s := &session{vals: map[string]*answer{}}
	return context.WithValue(ctx, sessionKey{}, s), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ended, s.vals = true, nil
	}
}

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
