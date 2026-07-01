package adapter

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRegistry_LazyOpenCallsOpenFnOnce(t *testing.T) {
	r := NewRegistry[int](
		8,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	var opens int32
	open := func() (int, error) {
		atomic.AddInt32(&opens, 1)
		return 42, nil
	}

	for i := 0; i < 5; i++ {
		v, release, err := r.Acquire("k", open)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		if v != 42 {
			t.Fatalf("value = %d, want 42", v)
		}
		release()
	}

	if got := atomic.LoadInt32(&opens); got != 1 {
		t.Fatalf("openFn called %d times, want 1", got)
	}
}

func TestRegistry_ReturnsCachedHandle(t *testing.T) {
	r := NewRegistry[*int](
		8,
		func(*int) error { return nil },
	)
	defer r.CloseAll()

	boxed := 7
	open := func() (*int, error) { return &boxed, nil }

	first, rel1, err := r.Acquire("k", open)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	second, rel2, err := r.Acquire(
		"k",
		func() (*int, error) {
			t.Fatalf("openFn must not be called for a cached key")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if first != second {
		t.Fatalf("cached handle pointer mismatch")
	}
	rel1()
	rel2()
}

func TestRegistry_ConcurrentSameKey_OpensOnce(t *testing.T) {
	r := NewRegistry[int](
		8,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	var opens int32
	start := make(chan struct{})
	open := func() (int, error) {
		atomic.AddInt32(&opens, 1)
		return 1, nil
	}

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, release, err := r.Acquire("same", open)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			release()
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&opens); got != 1 {
		t.Fatalf("openFn called %d times under concurrency, want 1", got)
	}
}

func TestRegistry_LRUEvictsAndClosesOldestUnpinned(t *testing.T) {
	var mu sync.Mutex
	closed := []string{}
	r := NewRegistry[string](
		2,
		func(v string) error {
			mu.Lock()
			closed = append(closed, v)
			mu.Unlock()
			return nil
		},
	)
	defer r.CloseAll()

	open := func(v string) func() (string, error) {
		return func() (string, error) { return v, nil }
	}

	_, relA, _ := r.Acquire("a", open("a"))
	relA()
	_, relB, _ := r.Acquire("b", open("b"))
	relB()
	_, relA2, _ := r.Acquire(
		"a",
		func() (string, error) {
			t.Fatalf("a should still be cached")
			return "", nil
		},
	)
	relA2()

	_, relC, _ := r.Acquire("c", open("c"))
	relC()

	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(closed) != 1 || closed[0] != "b" {
		t.Fatalf("closed = %v, want [b]", closed)
	}
}

func TestRegistry_PinnedEntryNotEvicted(t *testing.T) {
	var mu sync.Mutex
	closed := []string{}
	r := NewRegistry[string](
		1,
		func(v string) error {
			mu.Lock()
			closed = append(closed, v)
			mu.Unlock()
			return nil
		},
	)
	defer r.CloseAll()

	open := func(v string) func() (string, error) {
		return func() (string, error) { return v, nil }
	}

	_, relA, err := r.Acquire("a", open("a"))
	if err != nil {
		t.Fatalf("acquire a: %v", err)
	}

	_, relB, err := r.Acquire("b", open("b"))
	if err != nil {
		t.Fatalf("acquire b: %v", err)
	}

	mu.Lock()
	if len(closed) != 0 {
		mu.Unlock()
		t.Fatalf("pinned entry was closed: %v", closed)
	}
	mu.Unlock()
	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (over cap because both pinned)", r.Len())
	}

	relA()
	relB()
}

func TestRegistry_ReleaseAllowsEviction(t *testing.T) {
	var mu sync.Mutex
	closed := []string{}
	r := NewRegistry[string](
		1,
		func(v string) error {
			mu.Lock()
			closed = append(closed, v)
			mu.Unlock()
			return nil
		},
	)
	defer r.CloseAll()

	open := func(v string) func() (string, error) {
		return func() (string, error) { return v, nil }
	}

	_, relA, _ := r.Acquire("a", open("a"))
	relA()

	_, relB, _ := r.Acquire("b", open("b"))
	relB()

	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(closed) != 1 || closed[0] != "a" {
		t.Fatalf("closed = %v, want [a]", closed)
	}
}

func TestRegistry_CloseAll(t *testing.T) {
	var mu sync.Mutex
	closed := map[string]bool{}
	r := NewRegistry[string](
		8,
		func(v string) error {
			mu.Lock()
			closed[v] = true
			mu.Unlock()
			return nil
		},
	)

	open := func(v string) func() (string, error) {
		return func() (string, error) { return v, nil }
	}

	_, relA, _ := r.Acquire("a", open("a"))
	relA()
	_, relB, _ := r.Acquire("b", open("b"))
	relB()

	if err := r.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	if r.Len() != 0 {
		t.Fatalf("Len after CloseAll = %d, want 0", r.Len())
	}
	mu.Lock()
	defer mu.Unlock()
	if !closed["a"] || !closed["b"] {
		t.Fatalf("CloseAll did not close all handles: %v", closed)
	}
}

func TestRegistry_AcquirePropagatesOpenError(t *testing.T) {
	r := NewRegistry[int](
		8,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	sentinel := errors.New("boom")
	_, release, err := r.Acquire(
		"k",
		func() (int, error) { return 0, sentinel },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if release != nil {
		t.Fatalf("release should be nil on open error")
	}
	if r.Len() != 0 {
		t.Fatalf("Len = %d, want 0 after failed open", r.Len())
	}
}

func TestRegistry_NewRegistryDefaultsMaxOpen(t *testing.T) {
	r := NewRegistry[int](
		0,
		func(int) error { return nil },
	)
	defer r.CloseAll()
	if r.maxOpen != maxOpenEntityDBs {
		t.Fatalf("maxOpen = %d, want %d", r.maxOpen, maxOpenEntityDBs)
	}
}

func TestRegistry_CloseAllSkipsUnopenedReservation(t *testing.T) {
	var closes int32
	r := NewRegistry[int](
		8,
		func(int) error {
			atomic.AddInt32(&closes, 1)
			return nil
		},
	)

	gate := make(chan struct{})
	reserved := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, release, err := r.Acquire(
			"slow",
			func() (int, error) {
				close(reserved)
				<-gate
				return 9, nil
			},
		)
		if err != nil {
			t.Errorf("acquire: %v", err)
			return
		}
		release()
	}()

	<-reserved
	if err := r.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	if got := atomic.LoadInt32(&closes); got != 0 {
		t.Fatalf("closeFn called %d times for unopened reservation, want 0", got)
	}
	close(gate)
	<-done
}

func TestRegistry_ConcurrentOpenError_SecondAcquirerKeepsEntry(t *testing.T) {
	r := NewRegistry[int](
		8,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	sentinel := errors.New("first-fails")
	entry := r.reserve("k")
	entry.refcount++

	_, release, err := r.Acquire(
		"k",
		func() (int, error) { return 0, sentinel },
	)
	if !errors.Is(err, sentinel) {
		entry.refcount--
		t.Fatalf("err = %v, want sentinel", err)
	}
	if release != nil {
		t.Fatalf("release should be nil on open error")
	}
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (entry kept for second acquirer)", r.Len())
	}
	entry.refcount--
}

func TestRegistry_AbandonKeepsOpenedEntry(t *testing.T) {
	r := NewRegistry[int](
		8,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	entry := r.reserve("k")
	entry.opened = true
	entry.value = 5

	r.abandon(entry)

	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (opened entry must survive abandon)", r.Len())
	}
}

func TestRegistry_CloseAllAggregatesErrors(t *testing.T) {
	sentinel := errors.New("close-fail")
	r := NewRegistry[string](
		8,
		func(string) error { return sentinel },
	)

	_, rel, _ := r.Acquire(
		"a",
		func() (string, error) { return "a", nil },
	)
	rel()

	if err := r.CloseAll(); !errors.Is(err, sentinel) {
		t.Fatalf("CloseAll err = %v, want sentinel", err)
	}
}

func TestRegistry_EvictClosesAndRemovesEntry(t *testing.T) {
	var closed int32
	r := NewRegistry[int](
		8,
		func(int) error {
			atomic.AddInt32(&closed, 1)
			return nil
		},
	)
	defer r.CloseAll()

	var opens int32
	open := func() (int, error) {
		atomic.AddInt32(&opens, 1)
		return 1, nil
	}

	_, rel, err := r.Acquire("k", open)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	rel()

	if err := r.Evict("k"); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if got := atomic.LoadInt32(&closed); got != 1 {
		t.Fatalf("closeFn called %d times, want 1", got)
	}
	if r.Len() != 0 {
		t.Fatalf("Len after evict = %d, want 0", r.Len())
	}

	// Re-acquiring the same key re-opens it.
	if _, rel2, err := r.Acquire("k", open); err != nil {
		t.Fatalf("re-acquire: %v", err)
	} else {
		rel2()
	}
	if got := atomic.LoadInt32(&opens); got != 2 {
		t.Fatalf("openFn called %d times, want 2", got)
	}
}

func TestRegistry_EvictClosesPinnedEntry(t *testing.T) {
	var closed int32
	r := NewRegistry[int](
		8,
		func(int) error {
			atomic.AddInt32(&closed, 1)
			return nil
		},
	)
	defer r.CloseAll()

	// Acquire without releasing: the entry stays pinned (refcount > 0).
	if _, _, err := r.Acquire("k", func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := r.Evict("k"); err != nil {
		t.Fatalf("evict: %v", err)
	}
	if got := atomic.LoadInt32(&closed); got != 1 {
		t.Fatalf("evict must close even a pinned entry; closeFn called %d times", got)
	}
}

func TestRegistry_EvictAbsentKeyIsNoOp(t *testing.T) {
	r := NewRegistry[int](8, func(int) error { return nil })
	defer r.CloseAll()

	if err := r.Evict("nope"); err != nil {
		t.Fatalf("evict absent: %v", err)
	}
}
