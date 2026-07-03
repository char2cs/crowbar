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
// emission to the client sim, and asserts grid equality between them. It
// returns whether the emission demanded a keyframe, so callers that care
// which path served a given chunk (diff vs. keyframe redraw) can assert on
// it explicitly.
func conformanceStep(t *testing.T, m TerminalModel, e *DiffEmitter, ser Serializer, sim *clientSim, chunk []byte) (needKeyframe bool) {
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
	return needKeyframe
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

// TestConformance_MultiBatchScrollBurst pins writeScrollbackDelta's chunked
// write-then-scroll batching loop (base += rows) with a burst of >2x screen
// height of lines delivered in ONE chunk, arriving through the DIFF path
// rather than a keyframe redraw. A small first chunk primes the emitter so
// the burst is a real incremental Emit, not the initial unprimed keyframe
// that Serializer.Serialize would otherwise satisfy in one shot (which never
// touches writeScrollbackDelta's batching at all).
//
// conformanceStep's grid-equality check alone cannot pin this: after any
// scrollback growth, Emit nils the client's row cache and writeScreenDiff
// unconditionally repaints the whole viewport, so an incorrect (e.g.
// truncated) scrollback replay is invisible in the visible grid — the
// off-screen content it corrupts is the client's *scrollback buffer*, which
// gridString never renders. So this test additionally compares scrollback
// contents directly.
func TestConformance_MultiBatchScrollBurst(t *testing.T) {
	m, ser := newTestModel(t, 20, 6)
	sim := newClientSim(t, 20, 6, 200)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	// Prime: first Emit on an unprimed emitter always demands a keyframe, so
	// spend it here on something small.
	conformanceStep(t, m, e, ser, sim, []byte("start"))

	// 20 lines into a 6-row screen (>2x rows): forces writeScrollbackDelta's
	// base += rows loop through multiple iterations. Assert this is actually
	// served by the diff path, not silently upgraded to a keyframe redraw —
	// otherwise the batching loop below Emit is never exercised.
	needKeyframe := conformanceStep(t, m, e, ser, sim, []byte(scrollBurst(20)))
	require.False(t, needKeyframe,
		"scroll burst must be served by the diff path (writeScrollbackDelta), not a keyframe redraw")

	wantSB := scrollbackStrings(m)
	require.NotEmpty(t, wantSB, "burst must have committed scrollback lines to actually exercise the batching loop")
	require.Equal(t, wantSB, scrollbackStrings(sim.m),
		"client scrollback must match the model's after a multi-batch diff replay")
}

// TestConformance_RingFullStreaming pins Finding A: once x/vt's scrollback ring
// saturates, ScrollbackLen() plateaus at the cap, so the pre-fix delta path
// (which fired only on sbLen > lastLen) stopped forwarding scrolled-off lines to
// live clients entirely. A SMALL cap makes the ring saturate + rotate within a
// few Emits; the client sim carries the same cap so its own ring evicts
// identically — a correct implementation keeps the two scrollback contents
// equal across every rotating Emit. gridString equality (checked by
// conformanceStep) cannot catch this: the corrupted content is off-screen in
// the client's scrollback, which the visible grid never renders, so this
// asserts scrollbackStrings directly after each batch.
//
// Verified red-first: with scrollbackNewStart's rotation scan disabled (forced
// to the pre-fix `return sbLen, true` at plateau) the batch-3+ assertions fail.
func TestConformance_RingFullStreaming(t *testing.T) {
	const cols, rows, cap = 20, 4, 8
	m, ser := New(cols, rows, cap)
	t.Cleanup(func() { m.Close() })
	sim := newClientSim(t, cols, rows, cap)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	// Spend the mandatory initial keyframe on something small so the bursts
	// below are real incremental Emits through the diff path.
	conformanceStep(t, m, e, ser, sim, []byte("start"))

	for batch := 0; batch < 6; batch++ {
		var chunk strings.Builder
		for i := 0; i < 5; i++ {
			fmt.Fprintf(&chunk, "b%d-l%d\r\n", batch, i)
		}
		conformanceStep(t, m, e, ser, sim, []byte(chunk.String()))
		require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m),
			"client scrollback diverged from model after batch %d (ring rotation not tracked)", batch)
	}
	// Sanity: the ring must actually have saturated for this to have exercised
	// the plateau path at all.
	vm := m.(*vtModel)
	require.Equal(t, cap, vm.emu.ScrollbackLen(), "test setup: ring must be saturated at cap")
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
