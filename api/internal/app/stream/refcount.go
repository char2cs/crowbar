package stream

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type Resource interface {
	Start(key string) error
	Stop(key string)
}

type held struct {
	res Resource
	key string
}

type slot struct {
	holders int
	started chan struct{}
	err     error
}

type refcount struct {
	byKind map[stream.ScopeKind][]Resource

	mu       sync.Mutex
	slots    map[held]*slot
	stopping map[held]chan struct{}
}

func newRefcount(byKind map[stream.ScopeKind][]Resource) *refcount {
	return &refcount{byKind: byKind, slots: map[held]*slot{}, stopping: map[held]chan struct{}{}}
}

func (r *refcount) hold(scope stream.Scope) error {
	resources := r.byKind[scope.Kind]
	for i, res := range resources {
		if err := r.holdOne(held{res: res, key: scope.Key}); err != nil {
			for _, prev := range resources[:i] {
				r.releaseOne(held{res: prev, key: scope.Key})
			}
			return err
		}
	}
	return nil
}

func (r *refcount) release(scope stream.Scope) {
	for _, res := range r.byKind[scope.Kind] {
		r.releaseOne(held{res: res, key: scope.Key})
	}
}

func (r *refcount) holdOne(k held) error {
	r.mu.Lock()
	if s, ok := r.slots[k]; ok {
		s.holders++
		r.mu.Unlock()
		<-s.started
		return s.err
	}
	s := &slot{holders: 1, started: make(chan struct{})}
	r.slots[k] = s
	prev := r.stopping[k]
	r.mu.Unlock()

	if prev != nil {
		<-prev
	}
	s.err = k.res.Start(k.key)
	if s.err != nil {
		r.mu.Lock()
		if r.slots[k] == s {
			delete(r.slots, k)
		}
		r.mu.Unlock()
	}
	close(s.started)
	return s.err
}

func (r *refcount) releaseOne(k held) {
	r.mu.Lock()
	s := r.slots[k]
	s.holders--
	if s.holders > 0 {
		r.mu.Unlock()
		return
	}
	delete(r.slots, k)
	done := make(chan struct{})
	r.stopping[k] = done
	r.mu.Unlock()

	k.res.Stop(k.key)
	r.mu.Lock()
	if r.stopping[k] == done {
		delete(r.stopping, k)
	}
	r.mu.Unlock()
	close(done)
}
