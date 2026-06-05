package session

import (
	"sync"
	"testing"
)

// BenchmarkRingBuffer_WriteSnapshot benchmarks concurrent write + snapshot
// under the PTY fan-out hot path: one writer goroutine, N reader goroutines.
func BenchmarkRingBuffer_WriteSnapshot(b *testing.B) {
	r := newRingBuffer(defaultRingSize)
	data := []byte("output line from PTY\n")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Write(data)
			_ = r.Snapshot()
		}
	})
}

// BenchmarkRingBuffer_ConcurrentReadWrite benchmarks 8 concurrent readers vs
// a single background writer — reflects the actual fan-out pattern.
func BenchmarkRingBuffer_ConcurrentReadWrite(b *testing.B) {
	r := newRingBuffer(defaultRingSize)
	data := make([]byte, 256)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				r.Write(data)
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Snapshot()
	}
	b.StopTimer()

	close(stop)
	wg.Wait()
}
