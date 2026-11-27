package model

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	uv "github.com/charmbracelet/ultraviolet"
)

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
	fresh := snapshotRowInto(emu, nil, 10, 0)
	for x := range fresh {
		if !fresh[x].Equal(&refilled[x]) {
			t.Fatalf("reused row differs from a fresh snapshot at x=%d", x)
		}
	}
}

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

func TestEmit_ScrollingDoesNotReallocateViewportRows(t *testing.T) {
	const cols, rows = 80, 24
	m, _ := newTestModel(t, cols, rows)
	e := NewDiffEmitter()
	m.Write([]byte("seed"))
	e.Prime(m)

	line := make([]byte, 0, cols+2)
	for i := 0; i < cols; i++ {
		line = append(line, 'x')
	}
	line = append(line, '\r', '\n')

	const iterations = 200
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < iterations; i++ {
		m.Write(line)
		e.Emit(m)
	}
	runtime.ReadMemStats(&after)

	perScroll := float64(after.TotalAlloc-before.TotalAlloc) / iterations
	ceiling := float64(rows*cols) * float64(unsafe.Sizeof(uv.Cell{})) / 2
	if perScroll >= ceiling {
		t.Errorf("Emit allocates %.0f B per scrolled line; want < %.0f B "+
			"(the viewport's cell slices are being reallocated every emit — "+
			"has the scroll invalidation marker regressed to nil?)", perScroll, ceiling)
	}
	t.Logf("bytes allocated per scrolled line: %.0f (ceiling %.0f)", perScroll, ceiling)
}

func TestEmit_NoChangeWithChainsAllocatesNoMore(t *testing.T) {
	allocs := func(content string) float64 {
		m, _ := newTestModel(t, 80, 24)
		m.Write([]byte(content))
		e := NewDiffEmitter()
		e.Prime(m)
		if data, _ := e.Emit(m); len(data) > 0 {
			t.Fatalf("a no-change Emit wrote %q", data)
		}
		return testing.AllocsPerRun(100, func() { e.Emit(m) })
	}
	plain := allocs("hello")
	chained := allocs(strings.Repeat("x", 80*30))
	if chained > plain {
		t.Errorf("a no-change Emit allocates %.0f times holding chains, %.0f without", chained, plain)
	}
	t.Logf("no-change Emit allocations: %.0f with chains, %.0f without", chained, plain)
}
