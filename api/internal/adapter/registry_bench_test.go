package adapter

import (
	"strconv"
	"testing"
)

func BenchmarkRegistry_CacheHit(b *testing.B) {
	r := NewRegistry[int](
		64,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	open := func() (int, error) { return 1, nil }
	if _, rel, err := r.Acquire("k", open); err != nil {
		b.Fatalf("warm acquire: %v", err)
	} else {
		rel()
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, release, err := r.Acquire(
			"k",
			func() (int, error) {
				b.Fatalf("openFn must not run on cache hit")
				return 0, nil
			},
		)
		if err != nil {
			b.Fatalf("acquire: %v", err)
		}
		release()
	}
}

// BenchmarkRegistry_LazyOpenMiss measures the cold-open path: every iteration
// uses a fresh key, forcing openFn to run and the LRU to evict-and-recycle a
// slot so the cache stays bounded while the miss path is exercised repeatedly.
func BenchmarkRegistry_LazyOpenMiss(b *testing.B) {
	r := NewRegistry[int](
		64,
		func(int) error { return nil },
	)
	defer r.CloseAll()

	open := func() (int, error) { return 1, nil }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, release, err := r.Acquire(strconv.Itoa(i), open)
		if err != nil {
			b.Fatalf("acquire: %v", err)
		}
		release()
	}
}
