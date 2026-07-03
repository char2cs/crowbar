package model

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// clientSim mirrors a live client's terminal: a fresh model instance re-created
// on every keyframe (terminal.reset() equivalent) and Written to incrementally
// on every diff frame.
type clientSim struct {
	t          *testing.T
	cols, rows int
	sb         int
	m          TerminalModel
}

func newClientSim(t *testing.T, cols, rows, sb int) *clientSim {
	t.Helper()
	c := &clientSim{t: t, cols: cols, rows: rows, sb: sb}
	c.reset()
	return c
}

func (c *clientSim) reset() {
	if c.m != nil {
		c.m.Close()
	}
	m, _ := New(c.cols, c.rows, c.sb)
	c.m = m
}

func (c *clientSim) apply(data []byte, keyframe bool) {
	if keyframe {
		c.reset()
	}
	c.m.Write(data)
}

// gridString renders a model's visible grid plus cursor/alt-screen state as
// plain text for equality assertions. Content-level conformance only; SGR
// conformance is covered by the serializer's own tests since both paths share
// encodeLine.
func gridString(m TerminalModel) string {
	vm := m.(*vtModel)
	cols, rows := vm.emu.Width(), vm.emu.Height()
	var out strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if cell := vm.emu.CellAt(x, y); cell != nil {
				out.WriteString(cell.String())
			} else {
				out.WriteString(" ")
			}
		}
		out.WriteString("\n")
	}
	cur := vm.emu.CursorPosition()
	fmt.Fprintf(&out, "cursor=%d,%d alt=%v", cur.X, cur.Y, vm.emu.IsAltScreen())
	return out.String()
}

// conformanceStep feeds bytes to the authoritative model, emits, applies the
// emission to the client sim, and asserts grid equality between them.
func conformanceStep(t *testing.T, m TerminalModel, e *DiffEmitter, ser Serializer, sim *clientSim, chunk []byte) {
	t.Helper()
	m.Write(chunk)
	data, needKeyframe := e.Emit(m)
	if needKeyframe {
		redraw := ser.Serialize(m)
		sim.apply(redraw, true)
		e.Prime(m)
	} else if len(data) > 0 {
		sim.apply(data, false)
	}
	require.Equal(t, gridString(m), gridString(sim.m),
		"client grid diverged from model after chunk %q", string(chunk))
}

func TestConformance_ScriptedScenarios(t *testing.T) {
	scenarios := [][]string{
		{"hello", " world", "\r\nline2", "\x1b[2Jcleared", "\x1b[1;1Htop"},
		{"a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng"},                         // scroll-through
		{"\x1b[?1049h", "TUI FRAME", "\x1b[2;3Hmid", "\x1b[?1049l"}, // alt round-trip
		{"\x1b[31mred\x1b[0m plain", "\x1b[1;44mbold-on-blue\x1b[m"},
		{"\x1b]0;title-a\x07out", "\x1b[?2004h", "paste-mode-on"},
		{"tab\thandling", "\rcarriage", "\x1b[5Dback"},
		// DECSTBM: set a scroll region, write content that scrolls inside it,
		// then clear the region. Both the set and the clear force a keyframe
		// (Emit's scroll-region keyframe guard), so this exercises the
		// keyframe path specifically for regions.
		{
			"\x1b[2;4r",                           // set scroll region rows 2-4
			"\x1b[2;1Hr1\r\nr2\r\nr3\r\nr4\r\nr5", // scroll within region
			"\x1b[r",                              // clear scroll region
			"after-clear",
		},
		// DECOM (origin mode): set, write positioned content relative to the
		// (whole-screen) origin, unset. Set/unset both force a keyframe.
		{
			"\x1b[?6h",     // DECOM on
			"\x1b[2;3Hox",  // positioned write, origin-relative
			"\x1b[?6l",     // DECOM off
			"\x1b[1;1Hend", // positioned write, absolute again
		},
		// Multi-batch scroll burst: >2x screen height of lines delivered in ONE
		// chunk, pinning the chunked write-then-scroll scrollback-delta path
		// (writeScrollbackDelta batches by `rows`).
		{scrollBurst(20)}, // 20 lines into a 6-row screen (>2x rows)
		// Title + mode-flip mid-stream: chrome delta continuity across
		// multiple diff frames without an intervening keyframe.
		{
			"\x1b]0;first-title\x07",
			"\x1b[?2004h",
			"\x1b]0;second-title\x07",
			"\x1b[?25l", // hide cursor
			"\x1b[?2004l",
			"\x1b]0;third-title\x07",
			"\x1b[?25h", // show cursor
		},
	}
	for i, chunks := range scenarios {
		t.Run(fmt.Sprintf("scenario-%d", i), func(t *testing.T) {
			m, ser := newTestModel(t, 20, 6)
			sim := newClientSim(t, 20, 6, 200)
			t.Cleanup(func() { sim.m.Close() })
			e := NewDiffEmitter()
			for _, chunk := range chunks {
				conformanceStep(t, m, e, ser, sim, []byte(chunk))
			}
		})
	}
}

// scrollBurst builds n numbered lines ("l1\r\nl2\r\n...") as a single chunk.
func scrollBurst(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteString("\r\n")
		}
		fmt.Fprintf(&b, "line-%d", i)
	}
	return b.String()
}

func TestConformance_RandomizedByteSplits(t *testing.T) {
	// A fixed-seed random walk over printable text, cursor moves, SGR, line
	// feeds and occasional clears, delivered in adversarial split sizes.
	rng := rand.New(rand.NewSource(42))
	atoms := []string{
		"x", "yz", " ", "\r\n", "\x1b[1;1H", "\x1b[3;5H", "\x1b[31m", "\x1b[0m",
		"\x1b[2J", "\x1b[K", "long-run-of-text ", "\t", "\x1b[?25l", "\x1b[?25h",
	}
	m, ser := newTestModel(t, 24, 8)
	sim := newClientSim(t, 24, 8, 200)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	var pending []byte
	for i := 0; i < 400; i++ {
		pending = append(pending, atoms[rng.Intn(len(atoms))]...)
		// Deliver in random-sized chunks, sometimes splitting mid-sequence.
		for len(pending) > 0 {
			n := 1 + rng.Intn(len(pending))
			conformanceStep(t, m, e, ser, sim, pending[:n])
			pending = pending[n:]
			if rng.Intn(3) == 0 {
				break // leave a partial for the next atom to extend
			}
		}
	}
}
