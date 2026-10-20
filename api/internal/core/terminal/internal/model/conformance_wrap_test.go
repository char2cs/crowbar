package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newWrapConformance returns a 20x6 model, its client and a primed emitter: the first Emit is
// always a keyframe, so it is spent on a word before the chunks under test.
func newWrapConformance(
	t *testing.T,
) (TerminalModel, Serializer, *clientSim, *DiffEmitter) {
	t.Helper()
	m, ser := newTestModel(t, 20, 6)
	sim := newClientSim(t, 20, 6, 200)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()
	conformanceStep(t, m, e, ser, sim, []byte("start"))
	return m, ser, sim, e
}

func TestConformance_WrappedChainsUnderScroll(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)

	straddles := 0
	for i := 0; i < 30; i++ {
		line := strings.Repeat(string(rune('a'+i%26)), 25+i*8%21)
		require.False(t, conformanceStep(t, m, e, ser, sim, []byte("\r\n"+line)), "chunk %d takes the diff path", i)
		require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m), "scrollback after chunk %d", i)
		if asVT(t, m).emu.RowWrapped(0) {
			straddles++
		}
	}
	require.NotZero(t, straddles, "test setup: a chain must straddle the scrollback boundary")
}

// The head commits in one Emit and its tail in the next; between them the tail is edited in place
// on the top row, whose flag only the row above it could set.
func TestConformance_WrappedChainStraddlesScrollback(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)

	steps := []struct {
		name, chunk, flags string
	}{
		{
			name:  "the head commits, the tail stays on the top row",
			chunk: "\r\n0123456789012345678 xxxxx" + strings.Repeat("\r\n", 5),
			flags: "grid T F F F F F | scrollback F F",
		},
		{name: "the tail is edited on the top row", chunk: "\x1b[1;6Hmore\x1b[6;1H", flags: "grid T F F F F F | scrollback F F"},
		{name: "the tail commits", chunk: "\r\n", flags: "grid F F F F F F | scrollback F F T"},
		{
			name:  "a tail erased blank by EL 1 stays on the top row",
			chunk: strings.Repeat("y", 25) + "\x1b[1K" + strings.Repeat("\r\n", 5),
			flags: "grid T F F F F F | scrollback F F T F F F F F F",
		},
		{
			name:  "EL 2 ends the top row's wrap",
			chunk: "\x1b[1;1H\x1b[2Kplain\x1b[6;1H",
			flags: "grid F F F F F F | scrollback F F T F F F F F F",
		},
	}
	for _, s := range steps {
		require.False(t, conformanceStep(t, m, e, ser, sim, []byte(s.chunk)), "%s: the diff path", s.name)
		require.Equal(t, s.flags, wrapFlags(t, m), "test setup: %s", s.name)
		require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m), s.name)
	}
}

// One Emit commits more lines than the viewport holds, so the delta goes out in several batches;
// each batch must end where a chain does.
func TestConformance_WrappedBurstSpansBatches(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)

	var lines []string
	for _, n := range []int{25, 30, 45, 41, 35, 50, 3, 3, 3, 3, 3, 3} {
		lines = append(lines, strings.Repeat(string(rune('a'+len(lines))), n))
	}
	lines[1] = "0123456789012345678 " + lines[1][20:] // a head the client must be sent to its last column
	require.False(t, conformanceStep(t, m, e, ser, sim, []byte("\r\n"+strings.Join(lines, "\r\n"))), "the diff path")
	require.Equal(t, "grid F F F F F F | scrollback F F T F T F T T F T T F T F T T",
		wrapFlags(t, m), "test setup: chains cross every batch boundary")
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m))
}

// Chains scroll into history after the client's autowrap went off, through the top row and inside
// a batch; each head must still autowrap there.
func TestConformance_WrappedChainScrollsUnderAutowrapOff(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)

	for _, chunk := range []string{
		"\r\n0123456789abcdefghijKLMNO\r\nABCDEFGHIJklmnopqrstuvwxy",
		"\x1b[?7l",
		"\r\n\r\n\r\n", // the first tail stays on the top row
		"\r\n\r\n\r\n", // it commits with the second chain
	} {
		require.False(t, conformanceStep(t, m, e, ser, sim, []byte(chunk)), "%q takes the diff path", chunk)
		require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m), "scrollback after %q", chunk)
	}
	require.Equal(t, "grid F F F F F F | scrollback F F T F T", wrapFlags(t, m), "test setup")
}

// A chain taller than the viewport cannot go out whole in one write-then-scroll batch: it is cut at
// the viewport's height, a cut line loses its wrap bit, and every cell still arrives.
func TestConformance_WrappedChainTallerThanViewport(t *testing.T) {
	m, _, sim, e := newWrapConformance(t)

	steps := []struct {
		name, chunk, model, client string
	}{
		{
			name:   "fourteen lines of one chain commit at once",
			chunk:  "\r\n" + strings.Repeat("x", 400),
			model:  "grid T T T T T T | scrollback F F T T T T T T T T T T T T T",
			client: "grid T T T T T T | scrollback F F T T T T T F T T T T T F T",
		},
		{
			name:   "a viewport of lines commits and the top row still continues them",
			chunk:  strings.Repeat("y", 120),
			model:  "grid T T T T T T | scrollback F F T T T T T T T T T T T T T T T T T T T",
			client: "grid T T T T T T | scrollback F F T T T T T F T T T T T F T T T T T T F",
		},
	}
	for _, s := range steps {
		m.Write([]byte(s.chunk))
		data, needKeyframe := e.Emit(m)
		require.False(t, needKeyframe, "%s: a tall chain adds no keyframe trigger", s.name)
		sim.apply(data, false)
		require.Equal(t, gridString(m), gridString(sim.m), s.name)
		require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m), s.name)
		require.Equal(t, s.model, wrapFlags(t, m), "test setup: %s", s.name)
		require.Equal(t, s.client, wrapFlags(t, sim.m), "%s: only the cut lines lose their wrap bit", s.name)
	}
}

// A one-row viewport has no spare row to carry the top row's flag, so its chains lose their wrap
// bits; every cell still arrives, and no line the model keeps on screen is scrolled out.
func TestConformance_WrappedChainOnOneRow(t *testing.T) {
	m, ser := New(20, 1, 200)
	t.Cleanup(m.Close)
	sim := newClientSim(t, 20, 1, 200)
	t.Cleanup(func() { sim.m.Close() })
	e := NewDiffEmitter()
	conformanceStep(t, m, e, ser, sim, []byte("start"))

	m.Write([]byte("\r\n" + strings.Repeat("x", 45)))
	data, needKeyframe := e.Emit(m)
	require.False(t, needKeyframe)
	sim.apply(data, false)
	require.Equal(t, gridString(m), gridString(sim.m))
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m))
	require.Equal(t, "grid T | scrollback F F T", wrapFlags(t, m), "test setup")
	require.Equal(t, "grid F | scrollback F F F", wrapFlags(t, sim.m))
}
