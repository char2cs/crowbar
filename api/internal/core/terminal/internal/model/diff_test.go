package model

import (
	"fmt"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plainText concatenates each cell's content into a single string, mirroring
// cellContent's blank-cell handling in vt_serializer_test.go.
func plainText(cells []uv.Cell) string {
	var b strings.Builder
	for i := range cells {
		c := cells[i].Content
		if c == "" {
			c = " "
		}
		b.WriteString(c)
	}
	return b.String()
}

// scrollbackStrings renders a model's scrollback lines as trimmed plain text.
func scrollbackStrings(m TerminalModel) []string {
	vm := m.(*vtModel)
	n := vm.emu.ScrollbackLen()
	out := make([]string, 0, n)
	for y := 0; y < n; y++ {
		line := vm.emu.ScrollbackLine(y)
		cells := make([]uv.Cell, len(line))
		copy(cells, line)
		out = append(out, strings.TrimRight(plainText(cells), " "))
	}
	return out
}

// newTestModel mirrors vt_model_test.go's construction.
func newTestModel(t *testing.T, cols, rows int) (TerminalModel, Serializer) {
	t.Helper()
	m, s := New(cols, rows, 200)
	t.Cleanup(func() { m.Close() })
	return m, s
}

func TestDiffEmitter_UnprimedNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	data, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe, "an unprimed emitter must demand a keyframe")
	assert.Nil(t, data)
}

func TestDiffEmitter_NoChangeEmitsNothing(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	m.Write([]byte("hello"))
	e := NewDiffEmitter()
	e.Prime(m)
	data, needKeyframe := e.Emit(m)
	assert.False(t, needKeyframe)
	assert.Empty(t, data, "no model change → no bytes")
}

func TestDiffEmitter_DirtyLineRewritten(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	m.Write([]byte("hello"))
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("X")) // row 0 changes
	data, needKeyframe := e.Emit(m)
	require.False(t, needKeyframe)
	s := string(data)
	// Row 0 rewritten in place: absolute cursor position to row 1 col 1 + content.
	assert.Contains(t, s, "\x1b[1;1H", "dirty row must be addressed absolutely")
	assert.Contains(t, s, "helloX")
	// Rows 1-4 untouched: no addressing of row 2..5.
	assert.NotContains(t, s, "\x1b[2;1H")
}

func TestDiffEmitter_ResizeNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Resize(30, 5)
	_, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe, "dimension change must invalidate the diff base")
}

func TestDiffEmitter_InvalidateForcesKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	e.Invalidate()
	_, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe)
}

func TestDiffEmitter_AltScreenFlipNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?1049h")) // enter alt screen
	_, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe, "alt-screen flip must invalidate the diff base")
}

func TestDiffEmitter_PrimeAfterKeyframeResumesDiffing(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Resize(30, 5)
	_, need := e.Emit(m)
	require.True(t, need)
	e.Prime(m) // caller emitted the keyframe; emitter re-bases
	data, need := e.Emit(m)
	assert.False(t, need)
	assert.Empty(t, data)
}

func TestDiffEmitter_ScrollbackDeltaEmittedBeforeScreenDiff(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)

	// Write 5 numbered lines into a 3-row screen: 2+ lines commit to scrollback.
	m.Write([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive"))
	data, need := e.Emit(m)
	require.False(t, need)
	s := string(data)

	// Committed scrollback lines are painted onto the top rows and then
	// scrolled out (write-then-scroll) so the CLIENT gains them in its own
	// history.
	assert.Contains(t, s, "one")
	assert.Contains(t, s, "two")
	// The scrollback flow (which also uses "\x1b[1;1H" to paint its own top
	// row) must precede the final screen-diff repaint. Since both phases can
	// address row 1, detect the screen-diff repaint's CUP as the LAST
	// occurrence of "\x1b[1;1H" and confirm the scrollback content ("one")
	// appears before it.
	sbIdx := strings.Index(s, "one")
	screenIdx := strings.LastIndex(s, "\x1b[1;1H")
	require.GreaterOrEqual(t, screenIdx, 0)
	assert.Less(t, sbIdx, screenIdx, "scrollback delta must precede the final screen-diff repaint")
}

func TestDiffEmitter_NoScrollbackDeltaWhenNoneCommitted(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	m.Write([]byte("hi"))
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("!"))
	data, need := e.Emit(m)
	require.False(t, need)
	// Exactly one dirty row, no scroll flow (no bare "\n" scroll writes).
	assert.Equal(t, 1, strings.Count(string(data), "\x1b[1;1H"))
}

func TestDiffEmitter_ClientScrollbackReceivesCommittedContent(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	sim, _ := newTestModel(t, 10, 3) // client simulator
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive"))
	data, need := e.Emit(m)
	require.False(t, need)
	sim.Write(data)

	vmAuth := m.(*vtModel)
	simSB := scrollbackStrings(sim)
	require.Equal(t, vmAuth.emu.ScrollbackLen(), len(simSB), "sim must gain exactly the committed line count")
	for y := 0; y < vmAuth.emu.ScrollbackLen(); y++ {
		authLine := vmAuth.emu.ScrollbackLine(y)
		authCells := make([]uv.Cell, len(authLine))
		copy(authCells, authLine)
		authText := strings.TrimRight(plainText(authCells), " ")
		assert.Equal(t, authText, simSB[y], "scrollback line %d", y)
	}
}

func TestDiffEmitter_AltScreenSkipsScrollback(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	m.Write([]byte("\x1b[?1049h")) // alt screen
	_, need := e.Emit(m)
	require.True(t, need) // flip → keyframe
	e.Prime(m)
	m.Write([]byte("APP"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "APP")
}

// TestDiffEmitter_RotationAnchorLostForcesKeyframe pins M5(b): once the scrollback
// ring is SATURATED, scrollbackNewStart anchors on the previously-emitted tail's
// hash and scans back only rotationScanLimit lines. If a single emit gap commits
// MORE than that many distinct lines, the anchor is evicted past the scan window,
// scrollbackNewStart returns ok=false, and Emit must demand a keyframe rather than
// silently misplacing the new-line boundary.
func TestDiffEmitter_RotationAnchorLostForcesKeyframe(t *testing.T) {
	const capLines = 300
	m, _ := New(20, 5, capLines)
	t.Cleanup(func() { m.Close() })
	e := NewDiffEmitter()

	// Saturate the ring with distinct lines so ScrollbackLen plateaus at the cap.
	var fill []byte
	for i := 0; i < capLines+50; i++ {
		fill = append(fill, fmt.Sprintf("pre-%04d\r\n", i)...)
	}
	m.Write(fill)
	require.Equal(t, capLines, m.(*vtModel).emu.ScrollbackLen(), "test setup: ring must be saturated")
	e.Prime(m) // anchors on the current (pre-fill) tail

	// One gap commits > rotationScanLimit distinct lines, evicting the primed
	// anchor beyond the backward scan window.
	var gap []byte
	for i := 0; i < rotationScanLimit+50; i++ {
		gap = append(gap, fmt.Sprintf("post-%04d\r\n", i)...)
	}
	m.Write(gap)

	_, need := e.Emit(m)
	assert.True(t, need,
		"an anchor evicted past rotationScanLimit must force a keyframe resync, not a misanchored diff")
}

func TestDiffEmitter_ScrollRegionChangeNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 10, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[2;5r")) // DECSTBM
	_, need := e.Emit(m)
	assert.True(t, need, "a new scroll region invalidates the absolute-CUP diff base")
}

func TestDiffEmitter_OriginModeChangeNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 10, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?6h")) // DECOM origin mode
	_, need := e.Emit(m)
	assert.True(t, need, "origin mode invalidates the absolute-CUP diff base")
}

// A URL that filled a row and autowrapped must be written to the client as one flow.
func TestDiffEmitter_SoftWrappedURLWrittenUnbroken(t *testing.T) {
	m, _ := New(40, 6, 100)
	t.Cleanup(m.Close)
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("see " + wrapTestURL))
	data, needKeyframe := e.Emit(m)
	require.False(t, needKeyframe)
	assert.Contains(t, string(data), wrapTestURL)
}

// A program that filled a row exactly and then broke the line must not be joined.
func TestDiffEmitter_ProgramHardBreakNotJoined(t *testing.T) {
	m, _ := New(40, 6, 100)
	t.Cleanup(m.Close)
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("see " + wrapTestURL[:36] + "\r\n" + wrapTestURL[36:]))
	data, needKeyframe := e.Emit(m)
	require.False(t, needKeyframe)
	frame := string(data)
	assert.Contains(t, frame, wrapTestURL[:36])
	assert.Contains(t, frame, wrapTestURL[36:])
	assert.NotContains(t, frame, wrapTestURL)
}

// A chain goes out as one flow from its first row: CUP, EL 0 (clearing that row's wrap bit), each
// head padded to the last column, the tail trimmed then EL 0. A lone row keeps the plain form.
func TestDiffEmitter_ChainWrittenAsOneFlow(t *testing.T) {
	m, _ := newTestModel(t, 20, 6)
	m.Write([]byte(strings.Repeat("x", 25)))
	e := NewDiffEmitter()
	e.Prime(m)
	x20 := strings.Repeat("x", 20)

	frames := []struct {
		name, chunk, want string
	}{
		{name: "an edited tail", chunk: "\x1b[2;1Hab", want: "\x1b[1;1H\x1b[K" + x20 + "abxxx\x1b[K\x1b[2;3H"},
		{name: "autowrap turned off", chunk: "\x1b[?7l", want: "\x1b[?7l"},
		{
			name:  "a chain under the client's ?7l is bracketed",
			chunk: "\x1b[2;1Hcd",
			want:  "\x1b[?7h\x1b[1;1H\x1b[K" + x20 + "cdxxx\x1b[K\x1b[?7l\x1b[2;3H",
		},
		{name: "a lone row is not", chunk: "\x1b[4;1Hlone", want: "\x1b[4;1H\x1b[Klone\x1b[4;5H"},
	}
	for _, f := range frames {
		m.Write([]byte(f.chunk))
		data, needKeyframe := e.Emit(m)
		require.False(t, needKeyframe, f.name)
		require.Equal(t, f.want, string(data), f.name)
	}
}

// A wrap bit that changes under unchanged cells is written once; the client then holds it.
func TestDiffEmitter_WrapFlagFlipEmitsOnce(t *testing.T) {
	m, _ := newTestModel(t, 20, 6)
	m.Write([]byte(strings.Repeat("x", 20) + "\r\nyyy"))
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("\x1b[1;1H" + strings.Repeat("x", 20) + "yyy"))
	data, _ := e.Emit(m)
	require.Equal(t, "\x1b[1;1H\x1b[K"+strings.Repeat("x", 20)+"yyy\x1b[K\x1b[2;4H", string(data))
	data, _ = e.Emit(m)
	require.Empty(t, data, "the client already holds the flag")
}

// A chain whose flow crosses the bottom of a primed scroll region lifts the region for the flow and
// restores it, so the client's autowrap moves down a row instead of scrolling the region.
func TestDiffEmitter_ChainLiftsTheScrollRegionItCrosses(t *testing.T) {
	m, _ := newTestModel(t, 20, 6)
	m.Write([]byte("\r\n\r\n\r\n" + strings.Repeat("x", 25) + "\x1b[1;4r"))
	e := NewDiffEmitter()
	e.Prime(m)

	frames := []struct {
		name, chunk, want string
	}{
		{
			name:  "a chain across the region's bottom",
			chunk: "\x1b[5;3HZ",
			want:  "\x1b[;r\x1b[4;1H\x1b[K" + strings.Repeat("x", 20) + "xxZxx\x1b[K\x1b[1;4r\x1b[5;4H",
		},
		{
			name:  "a chain inside the region",
			chunk: "\x1b[1;1H" + strings.Repeat("y", 25),
			want:  "\x1b[1;1H\x1b[K" + strings.Repeat("y", 20) + "yyyyy\x1b[K\x1b[2;6H",
		},
	}
	for _, f := range frames {
		m.Write([]byte(f.chunk))
		data, needKeyframe := e.Emit(m)
		require.False(t, needKeyframe, f.name)
		require.Equal(t, f.want, string(data), f.name)
	}
}
