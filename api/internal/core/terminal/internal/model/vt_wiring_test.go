package model

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/x/vt"
)

// TestSerializeReconcilesAltDesyncSetsDegraded forces the §4 "Mismatch handling"
// path: the shadow alt-screen mirror drifts from the authoritative emulator value,
// and Serialize must reconcile the shadow to the emulator AND latch Degraded().
func TestSerializeReconcilesAltDesyncSetsDegraded(t *testing.T) {
	m, s := New(20, 5, 100)
	vm := m.(*vtModel)

	// Emulator is on the primary buffer; force the shadow to claim alt-screen.
	vm.shadow.altScreen = true
	if vm.Degraded() {
		t.Fatal("model must not be degraded before the desync is observed")
	}

	_ = s.Serialize(m)

	if !vm.Degraded() {
		t.Fatal("alt-screen shadow desync must set Degraded()")
	}
	if vm.shadow.altScreen {
		t.Fatal("shadow.altScreen must be reconciled to the emulator value (primary)")
	}

	// Idempotent: a second serialize on a now-consistent model does not flip anything.
	_ = s.Serialize(m)
	if vm.shadow.altScreen {
		t.Fatal("reconciled shadow must stay consistent across serializes")
	}

	// A SECOND, fresh desync still reconciles and keeps Degraded sticky, but takes the
	// already-warned branch (no duplicate log spam) — exercised here for completeness.
	vm.shadow.altScreen = true
	_ = s.Serialize(m)
	if vm.shadow.altScreen {
		t.Fatal("second desync must also reconcile the shadow to the emulator")
	}
	if !vm.Degraded() {
		t.Fatal("Degraded() must remain sticky after a second desync")
	}
}

// TestRoundTripScrollbackWithTrailingBlankRows pins the §6 boundary invariant for a
// grid whose cursor is parked ABOVE the bottom row, leaving trailing blank grid rows.
// If the serializer ever under-padded those trailing rows, the fresh emulator would
// push a different number of physical lines into scrollback — caught here directly.
func TestRoundTripScrollbackWithTrailingBlankRows(t *testing.T) {
	const cols, rows = 20, 6
	m, s := New(cols, rows, 1000)
	vm := m.(*vtModel)

	var in []byte
	for i := 0; i < 14; i++ {
		in = append(in, []byte(fmt.Sprintf("history %d\r\n", i))...)
	}
	// Clear the visible screen, home, then write two short lines so rows 2..5 stay
	// blank and the cursor parks on row 1 — above the bottom row.
	in = append(in, []byte("\x1b[2J\x1b[H")...)
	in = append(in, []byte("top one\r\ntop two")...)
	m.Write(in)

	if y := vm.emu.CursorPosition().Y; y >= rows-1 {
		t.Fatalf("setup: cursor row %d is not above the bottom row %d", y, rows-1)
	}
	sbLen := vm.emu.ScrollbackLen()
	if sbLen == 0 {
		t.Fatal("setup produced no scrollback; the trailing-blank-row guard is vacuous")
	}

	payload := s.Serialize(m)

	fresh := vt.NewEmulator(cols, rows)
	fresh.SetScrollbackSize(1000)
	if _, err := fresh.Write(payload); err != nil {
		t.Fatalf("fresh emulator write: %v", err)
	}
	if got := fresh.ScrollbackLen(); got != sbLen {
		t.Fatalf("scrollback depth mismatch: model %d, fresh %d — trailing blank grid rows "+
			"were under-padded (the grid must always emit all %d rows)", sbLen, got, rows)
	}
}

// TestModelBytesAccountsGridAndScrollback verifies ModelBytes counts both the live
// grid and the retained scrollback cells, so the engine memory ceiling is not
// undercounted as a session accumulates history.
func TestModelBytesAccountsGridAndScrollback(t *testing.T) {
	m, _ := New(80, 24, 1000)

	base := m.ModelBytes()
	if want := int64(80 * 24 * bytesPerCell); base != want {
		t.Fatalf("empty model bytes = %d, want %d", base, want)
	}

	var in []byte
	for i := 0; i < 30; i++ {
		in = append(in, []byte(fmt.Sprintf("line %d\r\n", i))...)
	}
	m.Write(in)

	if grown := m.ModelBytes(); grown <= base {
		t.Fatalf("model bytes must grow with scrollback: base=%d grown=%d", base, grown)
	}
}
