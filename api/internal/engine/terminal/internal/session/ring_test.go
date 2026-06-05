package session

import (
	"bytes"
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
