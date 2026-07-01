package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The tests in this file assert the ACTUAL serialized bytes produced for a known input fed
// through the real model.Write/Resize path (which drives x/vt's callbacks + the escan
// scanner exactly as production does). They deliberately do NOT validate by re-parsing the
// redraw into a fresh x/vt emulator: that round-trip shares x/vt's own blind spots (it
// silently re-derives application-keypad and re-clamps the scroll region) and would have
// hidden the two re-attach regressions these tests pin.

const (
	setDECCKM   = "\x1b[?1h" // DECSET ?1, application cursor keys
	resetDECCKM = "\x1b[?1l" // DECRST ?1
	deckpamSeq  = "\x1b="    // DECKPAM, application keypad
)

// TestRegression_SerializeReassertsDECCKM proves the headline BUG-1 symptom (arrow-key
// history dying after a workspace switch) cannot recur via a dropped DECCKM: an app that
// turned on application cursor keys has ?1 re-asserted byte-for-byte in the re-attach
// redraw, so the new client emits SS3 arrows the TUI reads as history nav.
func TestRegression_SerializeReassertsDECCKM(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(setDECCKM))
	out := serialize(m)
	if !strings.Contains(out, setDECCKM) {
		t.Fatalf("re-attach redraw dropped DECCKM (?1h); arrows would emit CSI, breaking history: %q", out)
	}
}

// TestRegression_SerializeOmitsDECCKMWhenOff is the negative half: a model that never
// enabled ?1 (or explicitly reset it) must NOT emit ?1h, so re-attach does not silently
// flip a default-numeric terminal into application-cursor mode.
func TestRegression_SerializeOmitsDECCKMWhenOff(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(setDECCKM))
	m.Write([]byte(resetDECCKM))
	out := serialize(m)
	if strings.Contains(out, setDECCKM) {
		t.Fatalf("re-attach redraw emitted ?1h despite a clean ?1l state: %q", out)
	}
}

// TestRegression_SerializeReassertsApplicationKeypad is the genuine BUG-1 defect: x/vt
// surfaces DECKPAM (ESC =) only as an EnableMode callback for DECMode(66), which the model
// previously ignored entirely, so application-keypad was lost on every re-attach. The
// redraw must re-emit ESC = (NOT a DECSET ?66h, which does not re-establish it on xterm).
// This FAILS before the fix (no keypad byte tracked or emitted) and passes after.
func TestRegression_SerializeReassertsApplicationKeypad(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(deckpamSeq)) // ESC = via the real x/vt callback path
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

// TestRegression_SerializeOmitsKeypadWhenNumeric is the negative half: numeric keypad
// (the default, or after ESC >) must NOT emit ESC = on re-attach.
func TestRegression_SerializeOmitsKeypadWhenNumeric(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte(deckpamSeq)) // ESC = on
	m.Write([]byte("\x1b>"))    // ESC > back to numeric
	if m.shadow.keypadApplication {
		t.Fatal("ESC > did not clear keypadApplication shadow flag")
	}
	out := serialize(m)
	if strings.Contains(out, deckpamSeq) {
		t.Fatalf("re-attach redraw emitted ESC = despite numeric keypad: %q", out)
	}
}

// TestRegression_SerializeScrollRegionAndCursorBytes pins the exact DECSTBM params and the
// exact final CUP for a Claude-Code-style layout: a scroll region across the transcript with
// the cursor parked on the input row BELOW it. Both the region and the absolute cursor must
// appear with their correct row/col, and the DECSTBM must precede the final CUP (the CUP
// restores the cursor the client's DECSTBM moved to home).
func TestRegression_SerializeScrollRegionAndCursorBytes(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[2;20r")) // DECSTBM top=2 bottom=20
	m.Write([]byte("\x1b[23;7H")) // park cursor on row 23, col 7 (input line, below region)
	out := serialize(m)

	wantRegion := ansi.SetTopBottomMargins(2, 20) // "\x1b[2;20r"
	wantCUP := ansi.CursorPosition(7, 23)         // col 7, row 23 -> "\x1b[23;7H"
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

// TestRegression_SerializeScrollRegionResetAfterResize is the concrete BUG-2 defect: a
// SIGWINCH (every workspace switch resizes the recreated xterm) makes x/vt reset its scroll
// region to full-screen, but the escan-tracked shadow region went stale, so the re-attach
// redraw re-emitted a DECSTBM the live app had already lost — landing its next repaint one
// row off and leaking footer chrome into the input line. After the fix, Resize clears the
// shadow region and the redraw emits no stale DECSTBM. FAILS before the fix.
func TestRegression_SerializeScrollRegionResetAfterResize(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[2;20r"))
	if !m.shadow.scrollRegionSet {
		t.Fatal("test setup: DECSTBM was not tracked")
	}
	m.Resize(80, 30) // SIGWINCH; x/vt resets its region to the full new bounds
	if m.shadow.scrollRegionSet {
		t.Fatal("Resize did not clear the stale shadow scroll region")
	}
	out := serialize(m)
	if strings.Contains(out, ansi.SetTopBottomMargins(2, 20)) {
		t.Fatalf("redraw re-emitted a stale DECSTBM after resize: %q", out)
	}
}

// TestRegression_SerializeStateResetAfterRIS proves the same staleness class for a RIS
// (ESC c) full reset, which x/vt processes by resetting the region, charset and keypad
// WITHOUT a callback for the un-surfaced pieces. The escan RIS handler clears them so the
// redraw carries no stale DECSTBM, charset designation, or application keypad. FAILS before
// the fix.
func TestRegression_SerializeStateResetAfterRIS(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[2;20r")) // scroll region
	m.Write([]byte("\x1b(0"))     // designate G0 = DEC line drawing
	m.Write([]byte(deckpamSeq))   // application keypad
	m.Write([]byte("\x1bc"))      // RIS

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

// TestRegression_SerializeKeepsKeypadAfterRISInSameChunk pins the applyRIS clobber: Write
// runs the emulator-callback pass over the whole chunk FIRST (observing ESC = -> keypad on)
// and the escan pass SECOND. A RIS handler that cleared keypadApplication would, for a chunk
// like "ESC c ESC =" (RIS then DECKPAM, as a terminfo rs1/rs2 + smkx batch can produce),
// stomp the keypad=true the emulator already observed — because escan never re-observes the
// trailing ESC =. x/vt's own resetModes already corrected DEC-66 to the pre-ESC-= default at
// the RIS, then the ESC = re-set it; so the only thing that could lose it is the escan clobber.
// This FAILS while applyRIS clears keypadApplication and passes once that line is removed.
func TestRegression_SerializeKeepsKeypadAfterRISInSameChunk(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1bc\x1b=")) // RIS then DECKPAM in ONE Write chunk
	if !m.shadow.keypadApplication {
		t.Fatal("application keypad set after a same-chunk RIS was clobbered by applyRIS")
	}
	out := serialize(m)
	if !strings.Contains(out, deckpamSeq) {
		t.Fatalf("re-attach redraw dropped application keypad set after a same-chunk RIS: %q", out)
	}
}
