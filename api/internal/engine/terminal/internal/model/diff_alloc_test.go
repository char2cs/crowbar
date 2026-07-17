package model

import (
	"runtime"
	"testing"
	"unsafe"

	uv "github.com/charmbracelet/ultraviolet"
)

// backing returns the address of s's backing array, or 0 when there is none.
// Comparing it across calls is what proves reuse rather than reallocation.
func backing(s []uv.Cell) uintptr {
	if cap(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[:1][0]))
}

func allocTestEmu(t *testing.T, cols, rows int) emulator {
	t.Helper()
	m, _ := newTestModel(t, cols, rows)
	m.Write([]byte("hello"))
	return m.(*vtModel).emu
}

func TestSnapshotRowInto_ReusesBackingArray(t *testing.T) {
	emu := allocTestEmu(t, 10, 3)

	first := snapshotRowInto(emu, nil, 10, 0)
	addr := backing(first)
	if addr == 0 {
		t.Fatal("first snapshot allocated nothing")
	}

	second := snapshotRowInto(emu, first, 10, 0)
	if backing(second) != addr {
		t.Error("snapshotRowInto reallocated instead of reusing a large-enough dst")
	}
	if len(second) != 10 {
		t.Errorf("len = %d, want 10", len(second))
	}
}

// A truncated row is the scroll invalidation marker Emit writes. It must keep
// its capacity (so the next snapshot reuses it) AND refill correctly. Reverting
// that marker to nil silently reallocates every row on every scroll.
func TestSnapshotRowInto_ReusesTruncatedRow(t *testing.T) {
	emu := allocTestEmu(t, 10, 3)

	row := snapshotRowInto(emu, nil, 10, 0)
	addr := backing(row)

	refilled := snapshotRowInto(emu, row[:0], 10, 0)
	if backing(refilled) != addr {
		t.Error("a truncated row was not reused — the scroll path will allocate every row, every emit")
	}
	if len(refilled) != 10 {
		t.Fatalf("len = %d, want 10", len(refilled))
	}
	// Reuse must not leave stale cells behind.
	fresh := snapshotRowInto(emu, nil, 10, 0)
	for x := range fresh {
		if !fresh[x].Equal(&refilled[x]) {
			t.Fatalf("reused row differs from a fresh snapshot at x=%d", x)
		}
	}
}

// The truncation marker must still force a rewrite, exactly as nil did.
func TestRowEqualsGrid_TruncatedRowForcesRewrite(t *testing.T) {
	emu := allocTestEmu(t, 10, 3)

	row := snapshotRowInto(emu, nil, 10, 0)
	if !rowEqualsGrid(emu, row, 10, 0) {
		t.Fatal("a fresh snapshot should compare equal to its own row")
	}
	if rowEqualsGrid(emu, row[:0], 10, 0) {
		t.Error("a truncated row must NOT compare equal — it is the forced-rewrite marker")
	}
	if rowEqualsGrid(emu, nil, 10, 0) {
		t.Error("a nil row must NOT compare equal")
	}
}

func TestSnapshotRowInto_GrowsWhenDstTooSmall(t *testing.T) {
	emu := allocTestEmu(t, 10, 3)

	got := snapshotRowInto(emu, make([]uv.Cell, 0, 4), 10, 0)
	if len(got) != 10 {
		t.Fatalf("len = %d, want 10", len(got))
	}
	fresh := snapshotRowInto(emu, nil, 10, 0)
	for x := range fresh {
		if !fresh[x].Equal(&got[x]) {
			t.Fatalf("grown row differs from a fresh snapshot at x=%d", x)
		}
	}
}

// encodeGridRow must stay equivalent to encoding a snapshot of the same row —
// the identity that let writeScreenDiff drop its second materialisation.
func TestEncodeGridRow_MatchesEncodedSnapshot(t *testing.T) {
	emu := allocTestEmu(t, 10, 3)

	want := encodeGridRow(emu, 10, 0)
	got := encodeLine(snapshotRowInto(emu, nil, 10, 0), 10, true)
	if got != want {
		t.Errorf("encodeLine(snapshot) = %q, want encodeGridRow = %q", got, want)
	}
}

// Scrolling is the hot path: every emit grows scrollback, which invalidates
// every row. The rows' backing arrays must survive that invalidation and be
// refilled in place — nilling them (or re-materialising a row per emit for the
// wire) reallocates the whole viewport on every scrolled line.
//
// This asserts the allocation behaviour of the real Emit path, so it fails if
// the marker regresses to nil or the snapshot/encode pair is split apart again.
func TestEmit_ScrollingDoesNotReallocateViewportRows(t *testing.T) {
	const cols, rows = 80, 24
	m, _ := newTestModel(t, cols, rows)
	e := NewDiffEmitter()
	m.Write([]byte("seed"))
	e.Prime(m) // Emit bails immediately on an unprimed emitter and does no work

	line := make([]byte, 0, cols+2)
	for i := 0; i < cols; i++ {
		line = append(line, 'x')
	}
	line = append(line, '\r', '\n')

	// Measure BYTES, not object count: what regressing the marker reintroduces
	// is a rows x cols cell-slice per emit (24*80*112B ~= 215KB), which barely
	// moves the object count but dominates the bytes.
	const iterations = 200
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < iterations; i++ {
		m.Write(line) // one full-width line => scroll => full invalidation
		e.Emit(m)
	}
	runtime.ReadMemStats(&after)

	perScroll := float64(after.TotalAlloc-before.TotalAlloc) / iterations
	// One reallocated viewport is rows*cols*sizeof(uv.Cell); stay well under it.
	ceiling := float64(rows*cols) * float64(unsafe.Sizeof(uv.Cell{})) / 2
	if perScroll >= ceiling {
		t.Errorf("Emit allocates %.0f B per scrolled line; want < %.0f B "+
			"(the viewport's cell slices are being reallocated every emit — "+
			"has the scroll invalidation marker regressed to nil?)", perScroll, ceiling)
	}
	t.Logf("bytes allocated per scrolled line: %.0f (ceiling %.0f)", perScroll, ceiling)
}
