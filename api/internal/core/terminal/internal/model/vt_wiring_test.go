package model

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/x/vt"
)

func TestSerializeReconcilesAltDesyncSetsDegraded(t *testing.T) {
	m, s := New(20, 5, 100)
	vm := m.(*vtModel)

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

	_ = s.Serialize(m)
	if vm.shadow.altScreen {
		t.Fatal("reconciled shadow must stay consistent across serializes")
	}

	vm.shadow.altScreen = true
	_ = s.Serialize(m)
	if vm.shadow.altScreen {
		t.Fatal("second desync must also reconcile the shadow to the emulator")
	}
	if !vm.Degraded() {
		t.Fatal("Degraded() must remain sticky after a second desync")
	}
}

func TestRoundTripScrollbackWithTrailingBlankRows(t *testing.T) {
	const cols, rows = 20, 6
	m, s := New(cols, rows, 1000)
	vm := m.(*vtModel)

	var in []byte
	for i := 0; i < 14; i++ {
		in = append(in, []byte(fmt.Sprintf("history %d\r\n", i))...)
	}
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
