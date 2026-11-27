package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const (
	setDECCKM   = "\x1b[?1h"
	resetDECCKM = "\x1b[?1l"
	deckpamSeq  = "\x1b="
)

func TestRegression_SerializeReassertsDECCKM(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(setDECCKM))
	out := serialize(m)
	if !strings.Contains(out, setDECCKM) {
		t.Fatalf("re-attach redraw dropped DECCKM (?1h); arrows would emit CSI, breaking history: %q", out)
	}
}

func TestRegression_SerializeOmitsDECCKMWhenOff(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(setDECCKM))
	m.Write([]byte(resetDECCKM))
	out := serialize(m)
	if strings.Contains(out, setDECCKM) {
		t.Fatalf("re-attach redraw emitted ?1h despite a clean ?1l state: %q", out)
	}
}

func TestRegression_SerializeReassertsApplicationKeypad(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(deckpamSeq))
	if !m.shadow.keypadApplication {
		t.Fatal("ESC = did not set keypadApplication shadow flag")
	}
	out := serialize(m)
	if !strings.Contains(out, deckpamSeq) {
		t.Fatalf("re-attach redraw dropped application keypad (ESC =): %q", out)
	}
	if strings.Contains(out, ansi.SetMode(ansi.DECMode(66))) {
		t.Fatal("application keypad must be re-emitted as ESC =, never as DECSET ?66h")
	}
}

func TestRegression_SerializeOmitsKeypadWhenNumeric(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(deckpamSeq))
	m.Write([]byte("\x1b>"))
	if m.shadow.keypadApplication {
		t.Fatal("ESC > did not clear keypadApplication shadow flag")
	}
	out := serialize(m)
	if strings.Contains(out, deckpamSeq) {
		t.Fatalf("re-attach redraw emitted ESC = despite numeric keypad: %q", out)
	}
}

func TestRegression_SerializeScrollRegionAndCursorBytes(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[2;20r"))
	m.Write([]byte("\x1b[23;7H"))
	out := serialize(m)

	wantRegion := ansi.SetTopBottomMargins(2, 20)
	wantCUP := ansi.CursorPosition(7, 23)
	if !strings.Contains(out, wantRegion) {
		t.Fatalf("redraw missing DECSTBM %q: %q", wantRegion, out)
	}
	if !strings.Contains(out, wantCUP) {
		t.Fatalf("redraw missing final CUP %q: %q", wantCUP, out)
	}
	if strings.Index(out, wantRegion) > strings.LastIndex(out, wantCUP) {
		t.Fatal("DECSTBM must be emitted before the final CUP, or the cursor lands at home")
	}
}

func TestRegression_SerializeScrollRegionResetAfterResize(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[2;20r"))
	if !m.shadow.scrollRegionSet {
		t.Fatal("test setup: DECSTBM was not tracked")
	}
	m.Resize(80, 30)
	if m.shadow.scrollRegionSet {
		t.Fatal("Resize did not clear the stale shadow scroll region")
	}
	out := serialize(m)
	if strings.Contains(out, ansi.SetTopBottomMargins(2, 20)) {
		t.Fatalf("redraw re-emitted a stale DECSTBM after resize: %q", out)
	}
}

func TestRegression_SerializeStateResetAfterRIS(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[2;20r"))
	m.Write([]byte("\x1b(0"))
	m.Write([]byte(deckpamSeq))
	m.Write([]byte("\x1bc"))

	if m.shadow.scrollRegionSet || m.shadow.g0 != 'B' || m.shadow.keypadApplication {
		t.Fatalf("RIS did not clear un-surfaced shadow state: region=%v g0=%q keypad=%v",
			m.shadow.scrollRegionSet, string(rune(m.shadow.g0)), m.shadow.keypadApplication)
	}
	out := serialize(m)
	if strings.Contains(out, ansi.SetTopBottomMargins(2, 20)) {
		t.Fatalf("redraw re-emitted a stale DECSTBM after RIS: %q", out)
	}
	if strings.Contains(out, deckpamSeq) {
		t.Fatalf("redraw re-emitted stale application keypad after RIS: %q", out)
	}
}

func TestRegression_SerializeKeepsKeypadAfterRISInSameChunk(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1bc\x1b="))
	if !m.shadow.keypadApplication {
		t.Fatal("application keypad set after a same-chunk RIS was clobbered by applyRIS")
	}
	out := serialize(m)
	if !strings.Contains(out, deckpamSeq) {
		t.Fatalf("re-attach redraw dropped application keypad set after a same-chunk RIS: %q", out)
	}
}
