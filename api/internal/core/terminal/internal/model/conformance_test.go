package model

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const wrapTestURL = "https://example.com/docs/guide/getting-started?ref=crowbar"

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

func (c *clientSim) resize(cols, rows int) {
	c.cols, c.rows = cols, rows
}

func (c *clientSim) apply(data []byte, keyframe bool) {
	if keyframe {
		c.reset()
	}
	c.m.Write(data)
}

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
	want, got := wrapFlags(t, m), wrapFlags(t, sim.m)
	if asVT(t, m).emu.IsAltScreen() {
		grid, _, _ := strings.Cut(want, " | ")
		want = grid + " | scrollback"
	}
	require.Equal(t, want, got, "client wrap flags diverged from model after chunk %q", string(chunk))
	return needKeyframe
}

func TestConformance_ScriptedScenarios(t *testing.T) {
	scenarios := [][]string{
		{"hello", " world", "\r\nline2", "\x1b[2Jcleared", "\x1b[1;1Htop"},
		{"a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng"},
		{"\x1b[?1049h", "TUI FRAME", "\x1b[2;3Hmid", "\x1b[?1049l"},
		{"\x1b[31mred\x1b[0m plain", "\x1b[1;44mbold-on-blue\x1b[m"},
		{"\x1b]0;title-a\x07out", "\x1b[?2004h", "paste-mode-on"},
		{"tab\thandling", "\rcarriage", "\x1b[5Dback"},
		{
			"\x1b[2;4r",
			"\x1b[2;1Hr1\r\nr2\r\nr3\r\nr4\r\nr5",
			"\x1b[r",
			"after-clear",
		},
		{
			"\x1b[?6h",
			"\x1b[2;3Hox",
			"\x1b[?6l",
			"\x1b[1;1Hend",
		},
		{
			"\x1b]0;first-title\x07",
			"\x1b[?2004h",
			"\x1b]0;second-title\x07",
			"\x1b[?25l",
			"\x1b[?2004l",
			"\x1b]0;third-title\x07",
			"\x1b[?25h",
		},
		{"see " + wrapTestURL[:20], wrapTestURL[20:]},
		{"0123456789012345678 abc", "\x1b[2;2HZ", "\x1b[1;1HZ"},
		{strings.Repeat("x", 20) + "\r\nyyy", "\x1b[1;1H" + strings.Repeat("x", 20) + "yyy"},
		{strings.Repeat("x", 25), "\x1b[2;1H\x1b[2Kxxxxx"},
		{strings.Repeat("0123456789", 5), "\x1b[2;5HZ"},
		{strings.Repeat("x", 25), "\x1b[?7l", "\x1b[2;1Habc", "\x1b[?7h\x1b[2;1Hdef"},
		{"\r\n\r\n\r\n" + strings.Repeat("x", 25) + "\x1b[1;4r", "\x1b[4;3HZ", "\x1b[5;3HZ"},
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

func TestConformance_MultiBatchScrollBurst(t *testing.T) {
	m, ser := newTestModel(t, 20, 6)
	sim := newClientSim(t, 20, 6, 200)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	conformanceStep(t, m, e, ser, sim, []byte("start"))

	needKeyframe := conformanceStep(t, m, e, ser, sim, []byte(scrollBurst(20)))
	require.False(t, needKeyframe,
		"scroll burst must be served by the diff path (writeScrollbackDelta), not a keyframe redraw")

	wantSB := scrollbackStrings(m)
	require.NotEmpty(t, wantSB, "burst must have committed scrollback lines to actually exercise the batching loop")
	require.Equal(t, wantSB, scrollbackStrings(sim.m),
		"client scrollback must match the model's after a multi-batch diff replay")
}

func TestConformance_RingFullStreaming(t *testing.T) {
	const cols, rows, cap = 20, 4, 8
	m, ser := New(cols, rows, cap)
	t.Cleanup(func() { m.Close() })
	sim := newClientSim(t, cols, rows, cap)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

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
	vm := m.(*vtModel)
	require.Equal(t, cap, vm.emu.ScrollbackLen(), "test setup: ring must be saturated at cap")
}

func TestConformance_RegionActiveScroll(t *testing.T) {
	const cols, rows = 20, 6
	m, ser := newTestModel(t, cols, rows)
	sim := newClientSim(t, cols, rows, 200)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	conformanceStep(t, m, e, ser, sim, []byte("\x1b[1;4r"))

	sawKeyframe := false
	for i := 0; i < 4; i++ {
		chunk := fmt.Sprintf("\x1b[1;1Hu%d-a\r\nu%d-b\r\nu%d-c\r\nu%d-d\r\nu%d-e\r\nu%d-f", i, i, i, i, i, i)
		if conformanceStep(t, m, e, ser, sim, []byte(chunk)) {
			sawKeyframe = true
		}
	}
	require.True(t, sawKeyframe,
		"a scroll under an active scroll region must force a keyframe (Finding B)")
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m),
		"client scrollback must match the model after the region-active scroll resync")
}

func TestConformance_UnsaturatedRingBlankTailMisanchor(t *testing.T) {
	const cols, rows, cap = 20, 4, 1000
	m, ser := New(cols, rows, cap)
	t.Cleanup(func() { m.Close() })
	sim := newClientSim(t, cols, rows, cap)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	conformanceStep(t, m, e, ser, sim, []byte("start"))

	needKeyframe := conformanceStep(t, m, e, ser, sim,
		[]byte("\r\np1\r\np2\r\np3\r\n\r\np5\r\np6\r\np7\r\np8"))
	require.False(t, needKeyframe, "setup batch must be served by the diff path")
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m),
		"setup batch: client scrollback must match the model")
	require.Less(t, m.(*vtModel).emu.ScrollbackLen(), cap, "test setup: ring must stay unsaturated")

	needKeyframe = conformanceStep(t, m, e, ser, sim,
		[]byte("\r\nf1\r\nf2\r\nf3\r\nf4\r\nb1\r\nb2\r\n\r\nD1\r\nD2\r\nD3\r\nD4"))
	require.False(t, needKeyframe, "probe batch must be served by the diff path")
	require.Less(t, m.(*vtModel).emu.ScrollbackLen(), cap, "test setup: ring must still be unsaturated")

	wantSB := scrollbackStrings(m)
	require.Len(t, wantSB, 16, "test setup: expected exactly 16 committed scrollback lines")
	require.Equal(t, wantSB, scrollbackStrings(sim.m),
		"client scrollback must track the model on an unsaturated ring even when the "+
			"newly committed batch ends in a line that duplicates the prior tail's content")
}

func TestConformance_AltScreenRoundTripOnSaturatedRing(t *testing.T) {
	const cols, rows, capLines = 20, 4, 8
	m, ser := New(cols, rows, capLines)
	t.Cleanup(func() { m.Close() })
	sim := newClientSim(t, cols, rows, capLines)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()

	conformanceStep(t, m, e, ser, sim, []byte("start"))

	var fill strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&fill, "pre-%02d\r\n", i)
	}
	conformanceStep(t, m, e, ser, sim, []byte(fill.String()))
	require.Equal(t, capLines, m.(*vtModel).emu.ScrollbackLen(),
		"test setup: ring must be saturated before the alt round-trip")
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m),
		"scrollback must match after saturating the ring")

	require.True(t, conformanceStep(t, m, e, ser, sim, []byte("\x1b[?1049h")),
		"entering alt must force a keyframe")
	conformanceStep(t, m, e, ser, sim, []byte("\x1b[2;3HALT-FRAME"))
	require.True(t, conformanceStep(t, m, e, ser, sim, []byte("\x1b[?1049l")),
		"exiting alt must force a keyframe")

	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m),
		"primary scrollback must survive the alt round-trip intact")

	var more strings.Builder
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&more, "post-%02d\r\n", i)
	}
	conformanceStep(t, m, e, ser, sim, []byte(more.String()))

	require.Equal(t, capLines, m.(*vtModel).emu.ScrollbackLen(), "ring must stay saturated")
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m),
		"client scrollback must equal the model's after an alt round-trip on a saturated ring")
}

func TestConformance_RandomizedByteSplits(t *testing.T) {
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
		for len(pending) > 0 {
			n := 1 + rng.Intn(len(pending))
			conformanceStep(t, m, e, ser, sim, pending[:n])
			pending = pending[n:]
			if rng.Intn(3) == 0 {
				break
			}
		}
	}
}
