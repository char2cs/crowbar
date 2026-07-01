package adapter

import (
	"container/list"
	"errors"
	"sync"
)

const maxOpenEntityDBs = 64

// Registry is a generic, concurrency-safe, ref-counted LRU cache of expensive
// handles (for example per-entity SQLite event-store and view DB handles).
//
// Handles are opened lazily under a per-key lock so that concurrent Acquire
// calls for the same key open the underlying handle exactly once. The cache is
// bounded by a maximum number of open handles; once that bound is exceeded the
// least-recently-used handle whose refcount is zero is closed via the closeFn
// provided to NewRegistry and lazily re-opened on the next Acquire. A handle
// that is currently in use (refcount greater than zero) is pinned and is never
// evicted, even if that temporarily pushes the cache over its bound.
type Registry[T any] struct {
	maxOpen int
	closeFn func(T) error

	mu      sync.Mutex
	entries map[string]*registryEntry[T]
	lru     *list.List
}

// NewRegistry constructs a Registry bounded by maxOpen open handles. closeFn is
// invoked to release a handle when it is evicted by the LRU policy or when
// CloseAll is called; it must be safe to call on a handle that is no longer in
// use. A non-positive maxOpen falls back to maxOpenEntityDBs.
func NewRegistry[T any](
	maxOpen int,
	closeFn func(T) error,
) *Registry[T] {
	if maxOpen <= 0 {
		maxOpen = maxOpenEntityDBs
	}
	return &Registry[T]{
		maxOpen: maxOpen,
		closeFn: closeFn,
		entries: make(map[string]*registryEntry[T]),
		lru:     list.New(),
	}
}

// Acquire returns the cached handle for key, lazily opening it via openFn on a
// miss. The returned handle is pinned until the returned release function is
// called, guaranteeing it will not be evicted or closed while in use. release
// must be called exactly once. On an open error the zero value, a nil release,
// and the wrapped error are returned and nothing is cached.
func (r *Registry[T]) Acquire(
	key string,
	openFn func() (T, error),
) (T, func(), error) {
	entry := r.reserve(key)

	entry.openMu.Lock()
	if entry.opened {
		entry.openMu.Unlock()
		return entry.value, r.releaserFor(entry), nil
	}

	value, err := openFn()
	if err != nil {
		entry.openMu.Unlock()
		r.abandon(entry)
		var zero T
		return zero, nil, err
	}

	entry.value = value
	entry.opened = true
	entry.openMu.Unlock()

	r.evictIfNeeded()
	return value, r.releaserFor(entry), nil
}

// CloseAll closes every cached handle via closeFn and empties the cache. Errors
// from individual closeFn calls are joined and returned together.
func (r *Registry[T]) CloseAll() error {
	r.mu.Lock()
	entries := make([]*registryEntry[T], 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}
	r.entries = make(map[string]*registryEntry[T])
	r.lru = list.New()
	r.mu.Unlock()

	var errs []error
	for _, e := range entries {
		if !e.opened {
			continue
		}
		if err := r.closeFn(e.value); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Evict removes the entry for key from the cache and closes its handle via
// closeFn, regardless of its refcount. It is a no-op if key is absent or not yet
// opened. Use it when the underlying resource is being destroyed (for example a
// deleted workspace whose DB files are about to be removed) so a stale handle is
// not left pinned. Callers must not use any handle previously returned for key
// after Evict.
func (r *Registry[T]) Evict(
	key string,
) error {
	r.mu.Lock()
	entry, ok := r.entries[key]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	r.lru.Remove(entry.elem)
	delete(r.entries, key)
	r.mu.Unlock()

	if !entry.opened {
		return nil
	}
	return r.closeFn(entry.value)
}

// Len reports the number of cached entries, including handles that are pinned
// or in the process of being opened.
func (r *Registry[T]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func (r *Registry[T]) reserve(
	key string,
) *registryEntry[T] {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, ok := r.entries[key]; ok {
		entry.refcount++
		r.lru.MoveToFront(entry.elem)
		return entry
	}

	entry := &registryEntry[T]{
		key:      key,
		refcount: 1,
	}
	entry.elem = r.lru.PushFront(entry)
	r.entries[key] = entry
	return entry
}

func (r *Registry[T]) abandon(
	entry *registryEntry[T],
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry.refcount--
	if entry.refcount > 0 || entry.opened {
		return
	}
	r.lru.Remove(entry.elem)
	delete(r.entries, entry.key)
}

func (r *Registry[T]) releaserFor(
	entry *registryEntry[T],
) func() {
	return func() {
		r.release(entry)
	}
}

func (r *Registry[T]) release(
	entry *registryEntry[T],
) {
	r.mu.Lock()
	entry.refcount--
	victim := r.pickVictimLocked()
	r.mu.Unlock()

	if victim == nil {
		return
	}
	_ = r.closeFn(victim.value)
}

func (r *Registry[T]) evictIfNeeded() {
	r.mu.Lock()
	victim := r.pickVictimLocked()
	r.mu.Unlock()

	if victim == nil {
		return
	}
	_ = r.closeFn(victim.value)
}

func (r *Registry[T]) pickVictimLocked() *registryEntry[T] {
	if len(r.entries) <= r.maxOpen {
		return nil
	}
	for elem := r.lru.Back(); elem != nil; elem = elem.Prev() {
		entry := elem.Value.(*registryEntry[T])
		if entry.refcount > 0 || !entry.opened {
			continue
		}
		r.lru.Remove(entry.elem)
		delete(r.entries, entry.key)
		return entry
	}
	return nil
}
