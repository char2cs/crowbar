package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestScanSCSDesignatesG0AndG1(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.scanCharsetAndRegion([]byte("\x1b(0")) // designate G0 = DEC line drawing
	if m.shadow.g0 != '0' {
		t.Fatalf("g0 = %q, want '0'", m.shadow.g0)
	}
	m.scanCharsetAndRegion([]byte("\x1b)A")) // designate G1 = UK
	if m.shadow.g1 != 'A' {
		t.Fatalf("g1 = %q, want 'A'", m.shadow.g1)
	}
}

func TestScanSCSMultiByteSkipsIntermediate(t *testing.T) {
	m := newVTModel(20, 6, 100)
	// ESC ( <intermediate 0x25 '%'> <final '5'>: a multi-byte designator; the final byte
	// is recorded, the intermediate skipped.
	m.scanCharsetAndRegion([]byte("\x1b(%5"))
	if m.shadow.g0 != '5' {
		t.Fatalf("g0 = %q, want final byte '5'", m.shadow.g0)
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not back in ground after multi-byte SCS: %d", m.escanState)
	}
}

func TestScanLockingShiftSIandSO(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.scanCharsetAndRegion([]byte{0x0e}) // SO -> GL = G1
	if m.shadow.glLock != 1 {
		t.Fatalf("glLock after SO = %d, want 1", m.shadow.glLock)
	}
	m.scanCharsetAndRegion([]byte{0x0f}) // SI -> GL = G0
	if m.shadow.glLock != 0 {
		t.Fatalf("glLock after SI = %d, want 0", m.shadow.glLock)
	}
}

func TestScanGroundIgnoresPlainBytes(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.scanCharsetAndRegion([]byte("plain text, no escapes"))
	if m.escanState != escGround || m.shadow.glLock != 0 || m.shadow.scrollRegionSet {
		t.Fatal("plain bytes mutated scanner/shadow state")
	}
}

func TestScanEscOtherSequenceReturnsToGround(t *testing.T) {
	m := newVTModel(20, 6, 100)
	// ESC M (reverse index) is not a sequence we track; it must drop us back to ground
	// without corrupting later parsing.
	m.scanCharsetAndRegion([]byte("\x1bM"))
	if m.escanState != escGround {
		t.Fatalf("ESC M left scanner in state %d", m.escanState)
	}
	m.scanCharsetAndRegion([]byte("\x1b(0"))
	if m.shadow.g0 != '0' {
		t.Fatal("scanner failed to recover after an untracked ESC sequence")
	}
}

func TestScanDECSTBMSetsRegion(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[2;7r"))
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 2 || m.shadow.scrollBottom != 7 {
		t.Fatalf("region = set:%v top:%d bottom:%d", m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMBareResetsRegion(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.shadow.scrollRegionSet = true
	m.shadow.scrollTop, m.shadow.scrollBottom = 2, 7
	m.scanCharsetAndRegion([]byte("\x1b[r")) // bare DECSTBM resets to full screen
	if m.shadow.scrollRegionSet || m.shadow.scrollTop != 0 || m.shadow.scrollBottom != 0 {
		t.Fatalf("bare DECSTBM did not reset region: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMTopOnlyDefaultsBottomToHeight(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[3r")) // top=3, bottom defaults to height (10)
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 3 || m.shadow.scrollBottom != 10 {
		t.Fatalf("top-only DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMEmptyTopDefaultsToOne(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[;6r")) // top empty -> 1, bottom=6
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 1 || m.shadow.scrollBottom != 6 {
		t.Fatalf("empty-top DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMPrivateMarkerIgnored(t *testing.T) {
	m := newVTModel(20, 10, 100)
	// CSI ? 1 ; 2 r is XTRESTORE (restore DEC private modes), NOT DECSTBM. The private
	// marker must suppress region handling.
	m.scanCharsetAndRegion([]byte("\x1b[?1;2r"))
	if m.shadow.scrollRegionSet {
		t.Fatal("private-marker CSI r was misread as DECSTBM")
	}
}

func TestScanDECSTBMInvalidIgnored(t *testing.T) {
	cases := []struct {
		name string
		seq  string
	}{
		{"top not numeric", "\x1b[x;5r"},
		{"bottom not numeric", "\x1b[2;yr"},
		{"top below one", "\x1b[0;5r"},
		{"bottom below one", "\x1b[2;0r"},
		{"top not below bottom", "\x1b[7;3r"},
		{"three params", "\x1b[1;2;3r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVTModel(20, 10, 100)
			m.scanCharsetAndRegion([]byte(tc.seq))
			if m.shadow.scrollRegionSet {
				t.Fatalf("invalid DECSTBM %q set a region", tc.seq)
			}
			if m.escanState != escGround {
				t.Fatalf("scanner not in ground after %q: %d", tc.seq, m.escanState)
			}
		})
	}
}

func TestScanNonDECSTBMCSIIgnored(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[5;9H")) // CUP, a non-'r' final
	if m.shadow.scrollRegionSet {
		t.Fatal("a non-DECSTBM CSI set a region")
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after CUP: %d", m.escanState)
	}
}

func TestScanCSIviaC1Introducer(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte{0x9b, '2', ';', '8', 'r'}) // 8-bit CSI ... DECSTBM
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 2 || m.shadow.scrollBottom != 8 {
		t.Fatalf("C1 CSI DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanCSIAbortedByCAN(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[2;7\x18r")) // CAN aborts mid-CSI; 'r' is plain
	if m.shadow.scrollRegionSet {
		t.Fatal("CAN-aborted CSI still applied a region")
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after CAN: %d", m.escanState)
	}
}

func TestScanCSIParamsCapped(t *testing.T) {
	m := newVTModel(20, 10, 100)
	// A pathological CSI with far more than maxEscanParams parameter bytes: accumulation
	// stops at the cap but the final byte still returns us to ground.
	long := "\x1b[" + strings.Repeat("9", maxEscanParams+100) + "H"
	m.scanCharsetAndRegion([]byte(long))
	if len(m.escanParams) > maxEscanParams {
		t.Fatalf("escanParams grew past cap: %d", len(m.escanParams))
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after capped CSI: %d", m.escanState)
	}
}

func TestScanSequenceSplitAcrossChunks(t *testing.T) {
	m := newVTModel(20, 10, 100)
	// DECSTBM split mid-sequence across three Writes must still be recognised.
	m.scanCharsetAndRegion([]byte("\x1b[2"))
	m.scanCharsetAndRegion([]byte(";6"))
	m.scanCharsetAndRegion([]byte("r"))
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 2 || m.shadow.scrollBottom != 6 {
		t.Fatalf("split DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
	// SCS split across the ESC / '(' / designator boundary.
	m2 := newVTModel(20, 10, 100)
	m2.scanCharsetAndRegion([]byte("\x1b"))
	m2.scanCharsetAndRegion([]byte("("))
	m2.scanCharsetAndRegion([]byte("0"))
	if m2.shadow.g0 != '0' {
		t.Fatalf("split SCS g0 = %q", m2.shadow.g0)
	}
}

func TestResetEscanReturnsToGround(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[2;7")) // leave scanner mid-CSI with buffered params
	if m.escanState != escCSI || len(m.escanParams) == 0 {
		t.Fatal("setup did not leave scanner mid-CSI")
	}
	m.resetEscan()
	if m.escanState != escGround || len(m.escanParams) != 0 || m.escanPrivate {
		t.Fatalf("resetEscan left state: %d params:%d priv:%v",
			m.escanState, len(m.escanParams), m.escanPrivate)
	}
}

// TestScanFeedsSerializerStep10And11 is the end-to-end proof that the in-Write scan makes
// the serializer's charset/locking-shift (step 10) and scroll-region (step 11) output LIVE
// in production: a real PTY-shaped Write of SCS + SO + DECSTBM bytes must surface in the
// serialized redraw, which it could not when those shadow fields stayed at defaults.
func TestScanFeedsSerializerStep10And11(t *testing.T) {
	m, s := New(40, 12, 100)
	// Designate G1 = DEC line drawing, invoke it via SO, and set a partial scroll region.
	m.Write([]byte("\x1b)0\x0e"))
	m.Write([]byte("\x1b[3;9r"))

	out := string(s.Serialize(m))

	if !strings.Contains(out, "\x1b)0") {
		t.Fatalf("G1 SCS designation not re-emitted: %q", out)
	}
	if !strings.Contains(out, "\x0e") {
		t.Fatalf("SO locking shift not re-emitted: %q", out)
	}
	if !strings.Contains(out, ansi.SetTopBottomMargins(3, 9)) {
		t.Fatalf("DECSTBM region not re-emitted: %q", out)
	}
}
