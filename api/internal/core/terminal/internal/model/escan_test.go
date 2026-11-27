package model

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func serializedOSC2(
	payload string,
) (string, bool) {
	const intro = "\x1b]2;"
	i := strings.Index(payload, intro)
	if i < 0 {
		return "", false
	}
	rest := payload[i+len(intro):]
	j := strings.Index(rest, "\x1b\\")
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

func TestOSCTitleUTF8WithC1STByte(t *testing.T) {
	const want = "✳ Claude Code"
	m, s := New(40, 12, 100)
	m.Write([]byte("\x1b]2;\xe2\x9c\xb3 Claude Code\x07"))

	if got := m.Title(); got != want {
		t.Fatalf("Title() = %q, want %q (truncated at the 0x9C C1-ST byte?)", got, want)
	}

	out := string(s.Serialize(m))
	payload, ok := serializedOSC2(out)
	if !ok {
		t.Fatalf("serialized redraw has no OSC 2 title: %q", out)
	}
	if payload != want {
		t.Fatalf("serialized OSC 2 payload = %q, want %q", payload, want)
	}
	if !utf8.ValidString(payload) {
		t.Fatalf("serialized OSC 2 payload is not valid UTF-8: %x", payload)
	}
}

func TestOSCTitleTerminators(t *testing.T) {
	cases := []struct {
		name string
		seq  string
	}{
		{"BEL", "\x1b]2;\xe2\x9c\xb3 Claude Code\x07"},
		{"7-bit ST", "\x1b]2;\xe2\x9c\xb3 Claude Code\x1b\\"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVTModel(40, 12, 100)
			m.Write([]byte(tc.seq))
			if got := m.Title(); got != "✳ Claude Code" {
				t.Fatalf("Title() = %q, want %q", got, "✳ Claude Code")
			}
		})
	}
}

func TestOSCTitleSplitAcrossWrites(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]2;\xe2\x9c"))
	m.Write([]byte("\xb3 Claude\x07"))
	if got := m.Title(); got != "✳ Claude" {
		t.Fatalf("split-across-writes Title() = %q, want %q", got, "✳ Claude")
	}
}

func TestOSCIconNameCaptured(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]1;myicon\x07"))
	if m.shadow.iconName != "myicon" {
		t.Fatalf("iconName = %q, want %q", m.shadow.iconName, "myicon")
	}
	if m.Title() != "" {
		t.Fatalf("OSC 1 must not set the title; Title() = %q", m.Title())
	}
}

func TestOSCZeroSetsBothTitleAndIcon(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]0;both\x1b\\"))
	if m.shadow.title != "both" || m.shadow.iconName != "both" {
		t.Fatalf("OSC 0 title/icon = %q/%q, want both %q", m.shadow.title, m.shadow.iconName, "both")
	}
}

func TestOSCPlainASCIITitle(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]2;claude\x07"))
	if got := m.Title(); got != "claude" {
		t.Fatalf("plain ASCII Title() = %q, want %q", got, "claude")
	}
}

func TestOSCTitlePreservesHighBytes(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]2;\xd0\x9d\x07"))
	if got := m.Title(); got != "Н" {
		t.Fatalf("Title() = %q, want %q", got, "Н")
	}
}

func TestOSCNonTitleSequencesIgnored(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]2;real\x07"))
	cases := []string{
		"\x1b]52;c;YWJj\x07",
		"\x1b]8;;http://x\x07",
		"\x1b]abc\x07",
		"\x1b]x;foo\x07",
	}
	for _, seq := range cases {
		m.Write([]byte(seq))
		if got := m.Title(); got != "real" {
			t.Fatalf("after %q, Title() = %q, want unchanged %q", seq, got, "real")
		}
	}
}

func TestOSCAbortedByCAN(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]2;first\x07"))
	m.Write([]byte("\x1b]2;discarded\x18"))
	if got := m.Title(); got != "first" {
		t.Fatalf("CAN-aborted OSC changed the title to %q, want %q", got, "first")
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after CAN: %d", m.escanState)
	}
}

func TestOSCEscNonBackslashStaysInString(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]2;ab\x1bXcd\x07"))
	if got := m.Title(); got != "abXcd" {
		t.Fatalf("Title() = %q, want %q (ESC dropped, X kept)", got, "abXcd")
	}
}

func TestOSCBodyCapped(t *testing.T) {
	m := newVTModel(40, 12, 100)
	long := "\x1b]2;" + strings.Repeat("a", maxEscanOSCText+500) + "\x07"
	m.Write([]byte(long))
	if len(m.escanOSC) > maxEscanOSCText {
		t.Fatalf("escanOSC grew past cap: %d", len(m.escanOSC))
	}
	if m.Title() == "" {
		t.Fatal("capped OSC title unexpectedly empty")
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after capped OSC: %d", m.escanState)
	}
}

func TestResetEscanClearsOSC(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.scanCharsetAndRegion([]byte("\x1b]2;partial"))
	if m.escanState != escOSC || len(m.escanOSC) == 0 {
		t.Fatal("setup did not leave scanner mid-OSC")
	}
	m.resetEscan()
	if m.escanState != escGround || len(m.escanOSC) != 0 {
		t.Fatalf("resetEscan left OSC state: %d osc:%d", m.escanState, len(m.escanOSC))
	}
}

func TestScanSCSDesignatesG0AndG1(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.scanCharsetAndRegion([]byte("\x1b(0"))
	if m.shadow.g0 != '0' {
		t.Fatalf("g0 = %q, want '0'", m.shadow.g0)
	}
	m.scanCharsetAndRegion([]byte("\x1b)A"))
	if m.shadow.g1 != 'A' {
		t.Fatalf("g1 = %q, want 'A'", m.shadow.g1)
	}
}

func TestScanSCSMultiByteSkipsIntermediate(t *testing.T) {
	m := newVTModel(20, 6, 100)
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
	m.scanCharsetAndRegion([]byte{0x0e})
	if m.shadow.glLock != 1 {
		t.Fatalf("glLock after SO = %d, want 1", m.shadow.glLock)
	}
	m.scanCharsetAndRegion([]byte{0x0f})
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
	m.scanCharsetAndRegion([]byte("\x1b[r"))
	if m.shadow.scrollRegionSet || m.shadow.scrollTop != 0 || m.shadow.scrollBottom != 0 {
		t.Fatalf("bare DECSTBM did not reset region: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMTopOnlyDefaultsBottomToHeight(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[3r"))
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 3 || m.shadow.scrollBottom != 10 {
		t.Fatalf("top-only DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMEmptyTopDefaultsToOne(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[;6r"))
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 1 || m.shadow.scrollBottom != 6 {
		t.Fatalf("empty-top DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
}

func TestScanDECSTBMPrivateMarkerIgnored(t *testing.T) {
	m := newVTModel(20, 10, 100)
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
	m.scanCharsetAndRegion([]byte("\x1b[5;9H"))
	if m.shadow.scrollRegionSet {
		t.Fatal("a non-DECSTBM CSI set a region")
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after CUP: %d", m.escanState)
	}
}

func TestScanC1CSIIntroducerIsUTF8Content(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.Write([]byte("\xe2\x80\x9b\x1b]2;hello\x07"))
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after C1-byte + OSC: %d", m.escanState)
	}
	if got := m.Title(); got != "hello" {
		t.Fatalf("Title() = %q, want %q (0x9B mis-framed as CSI swallowed the OSC?)", got, "hello")
	}
	if m.shadow.scrollRegionSet {
		t.Fatal("0x9B mis-framing synthesized a bogus scroll region")
	}
}

func TestScanCSIAbortedByESC(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[2;7\x1b(0"))
	if m.shadow.scrollRegionSet {
		t.Fatal("ESC-aborted CSI still applied a region")
	}
	if m.shadow.g0 != '0' {
		t.Fatalf("SCS after mid-CSI ESC not recognised: g0 = %q", m.shadow.g0)
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after ESC-aborted CSI + SCS: %d", m.escanState)
	}
}

func TestScanCSIAbortedByCAN(t *testing.T) {
	m := newVTModel(20, 10, 100)
	m.scanCharsetAndRegion([]byte("\x1b[2;7\x18r"))
	if m.shadow.scrollRegionSet {
		t.Fatal("CAN-aborted CSI still applied a region")
	}
	if m.escanState != escGround {
		t.Fatalf("scanner not in ground after CAN: %d", m.escanState)
	}
}

func TestScanCSIParamsCapped(t *testing.T) {
	m := newVTModel(20, 10, 100)
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
	m.scanCharsetAndRegion([]byte("\x1b[2"))
	m.scanCharsetAndRegion([]byte(";6"))
	m.scanCharsetAndRegion([]byte("r"))
	if !m.shadow.scrollRegionSet || m.shadow.scrollTop != 2 || m.shadow.scrollBottom != 6 {
		t.Fatalf("split DECSTBM: set:%v top:%d bottom:%d",
			m.shadow.scrollRegionSet, m.shadow.scrollTop, m.shadow.scrollBottom)
	}
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
	m.scanCharsetAndRegion([]byte("\x1b[2;7"))
	if m.escanState != escCSI || len(m.escanParams) == 0 {
		t.Fatal("setup did not leave scanner mid-CSI")
	}
	m.resetEscan()
	if m.escanState != escGround || len(m.escanParams) != 0 || m.escanPrivate {
		t.Fatalf("resetEscan left state: %d params:%d priv:%v",
			m.escanState, len(m.escanParams), m.escanPrivate)
	}
}

func TestScanFeedsSerializerStep10And11(t *testing.T) {
	m, s := New(40, 12, 100)
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
