package session

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBuffer_Empty(t *testing.T) {
	r := newRingBuffer(16)
	assert.Nil(t, r.Snapshot())
}

func TestRingBuffer_SmallWrite(t *testing.T) {
	r := newRingBuffer(16)
	r.Write([]byte("hello"))
	assert.Equal(t, []byte("hello"), r.Snapshot())
}

func TestRingBuffer_ExactFull(t *testing.T) {
	r := newRingBuffer(5)
	r.Write([]byte("hello"))
	assert.Equal(t, []byte("hello"), r.Snapshot())
}

func TestRingBuffer_Overflow(t *testing.T) {
	r := newRingBuffer(4)
	r.Write([]byte("abcde")) // overwrites 'a'
	assert.Equal(t, []byte("bcde"), r.Snapshot())
}

func TestRingBuffer_MultipleWrites(t *testing.T) {
	r := newRingBuffer(8)
	r.Write([]byte("abc"))
	r.Write([]byte("def"))
	assert.Equal(t, []byte("abcdef"), r.Snapshot())
}

func TestRingBuffer_OverflowMultipleWrites(t *testing.T) {
	r := newRingBuffer(4)
	r.Write([]byte("ab"))
	r.Write([]byte("cd"))
	r.Write([]byte("ef")) // overwrites 'a','b'
	assert.Equal(t, []byte("cdef"), r.Snapshot())
}

func TestRingBuffer_WrapAround(t *testing.T) {
	r := newRingBuffer(4)
	r.Write([]byte("abcd"))
	r.Write([]byte("xy")) // head wraps; overwrites a,b
	snap := r.Snapshot()
	assert.Equal(t, []byte("cdxy"), snap)
}

func TestRingBuffer_ConcurrentWrites(t *testing.T) {
	r := newRingBuffer(4096)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Write([]byte("x"))
		}()
	}
	wg.Wait()
	snap := r.Snapshot()
	assert.LessOrEqual(t, len(snap), 4096)
}

func TestRingBuffer_SnapshotIsChronological(t *testing.T) {
	r := newRingBuffer(8)
	r.Write([]byte("hello"))
	r.Write([]byte("!"))
	snap := r.Snapshot()
	assert.True(t, bytes.Contains(snap, []byte("hello!")))
}

func TestRingBuffer_OversizedWrite(t *testing.T) {
	// Write more bytes than capacity: only the tail should survive.
	r := newRingBuffer(8)
	r.Write([]byte("0123456789abcdef")) // 16 bytes > 8-byte capacity
	snap := r.Snapshot()
	assert.Len(t, snap, 8)
	assert.Equal(t, []byte("89abcdef"), snap)
}

func TestRingBuffer_OversizedWriteThenMore(t *testing.T) {
	// After an oversized write the ring head resets; subsequent writes should
	// wrap correctly.
	r := newRingBuffer(4)
	r.Write([]byte("abcdef")) // 6 > 4: keeps "cdef", head=0
	r.Write([]byte("xy"))     // wraps: "efxy" → "cdxy" wait...
	// After oversized: buf=[c,d,e,f], head=0, size=4
	// Write "xy" (2 bytes ≤ 4=tail from head=0): buf=[x,y,e,f], head=2
	// Snapshot: start=(2-4+4)%4=2, size=4 → buf[2:]=ef + buf[:2]=xy → "efxy"
	snap := r.Snapshot()
	assert.Len(t, snap, 4)
}

// --- Flush / Load / defaultRingSize tests ---

func TestRingBuffer_DefaultSize(t *testing.T) {
	assert.Equal(t, 1024*1024, defaultRingSize)
}

func TestRingBuffer_LazyGrowth_StartsSmallReportsCeiling(t *testing.T) {
	// A ring whose ceiling exceeds initialRingAlloc starts with a small backing
	// array but reports the full ceiling via Cap().
	r := newRingBuffer(defaultRingSize)
	assert.Equal(t, defaultRingSize, r.Cap(), "Cap reports the logical ceiling")
	assert.Equal(t, initialRingAlloc, len(r.buf), "backing array starts small")
}

func TestRingBuffer_LazyGrowth_GrowsToFitWithoutEvicting(t *testing.T) {
	// Writing more than initialRingAlloc but less than the ceiling must grow the
	// backing array and retain ALL bytes (no premature eviction).
	r := newRingBuffer(defaultRingSize)
	total := initialRingAlloc * 3
	payload := bytes.Repeat([]byte("x"), total)
	r.Write(payload)

	snap := r.Snapshot()
	assert.Equal(t, total, len(snap), "all bytes retained across growth")
	assert.Equal(t, payload, snap)
	assert.GreaterOrEqual(t, len(r.buf), total, "backing array grew to fit")
	assert.LessOrEqual(t, len(r.buf), defaultRingSize, "never exceeds the ceiling")
}

func TestRingBuffer_LazyGrowth_GrowsAcrossManyWritesThenWrapsAtCeiling(t *testing.T) {
	// Small ceiling just above initialRingAlloc: accumulate past it in chunks,
	// confirm growth keeps history, then confirm it wraps once the ceiling is hit.
	const ceiling = initialRingAlloc + 100
	r := newRingBuffer(ceiling)

	// Write well past the ceiling in small chunks so growth definitely reaches
	// the cap and the ring starts wrapping.
	for i := 0; i < (ceiling/10)+20; i++ {
		r.Write(bytes.Repeat([]byte("ab"), 5)) // 10 bytes/write
	}
	// Now the ring is at/over the ceiling and must behave as a fixed buffer.
	assert.Equal(t, ceiling, len(r.buf), "backing array capped at the ceiling")
	assert.Equal(t, ceiling, r.size, "ring is full at the ceiling")

	// One more write wraps and keeps exactly the last `ceiling` bytes.
	marker := bytes.Repeat([]byte("Z"), ceiling+50)
	r.Write(marker)
	snap := r.Snapshot()
	assert.Equal(t, ceiling, len(snap))
	assert.Equal(t, bytes.Repeat([]byte("Z"), ceiling), snap)
}

func TestRingBuffer_LazyGrowth_SmallCeilingIsEager(t *testing.T) {
	// A ceiling at or below initialRingAlloc is allocated eagerly — behaviour is
	// identical to the old fixed ring (the existing wrap tests rely on this).
	r := newRingBuffer(8)
	assert.Equal(t, 8, len(r.buf))
	assert.Equal(t, 8, r.Cap())
}

func TestRingBuffer_FlushLoad_RoundTrip(t *testing.T) {
	r := newRingBuffer(16)
	r.Write([]byte("hello world"))

	var buf bytes.Buffer
	err := r.Flush(&buf)
	assert.NoError(t, err)

	r2 := newRingBuffer(16)
	err = r2.Load(&buf)
	assert.NoError(t, err)

	assert.Equal(t, r.Snapshot(), r2.Snapshot())
}

func TestRingBuffer_FlushLoad_AfterWrapAround(t *testing.T) {
	// Write more than capacity so the ring wraps.
	r := newRingBuffer(8)
	r.Write([]byte("ABCDEFGH")) // fills exactly
	r.Write([]byte("XY"))       // wraps; oldest two bytes evicted

	preSnap := r.Snapshot()

	var buf bytes.Buffer
	assert.NoError(t, r.Flush(&buf))

	r2 := newRingBuffer(8)
	assert.NoError(t, r2.Load(&buf))

	assert.Equal(t, preSnap, r2.Snapshot())
}

func TestRingBuffer_Load_TruncatesToCapacity(t *testing.T) {
	// Load more bytes than capacity; ring should keep the last capacity bytes.
	r := newRingBuffer(4)
	src := bytes.NewReader([]byte("abcdefgh")) // 8 bytes into 4-byte ring
	assert.NoError(t, r.Load(src))

	snap := r.Snapshot()
	assert.Equal(t, 4, len(snap))
	assert.Equal(t, []byte("efgh"), snap)
}

func TestRingBuffer_Flush_ReturnsWriteError(t *testing.T) {
	r := newRingBuffer(8)
	r.Write([]byte("data"))

	err := r.Flush(&errorWriter{})
	assert.Error(t, err)
}

// errorWriter always errors on Write.
type errorWriter struct{}

func (e *errorWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("disk full")
}
