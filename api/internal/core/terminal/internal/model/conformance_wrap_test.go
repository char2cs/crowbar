package model

import (
	"fmt"
	"math/rand"
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
	// The client's row 0 never gains its flag, so every Emit rewrites it while it continues a line.
	require.Equal(t, "grid F | scrollback F F F", wrapFlags(t, sim.m))
}

// A seeded random walk on small viewports. After every Emit the client's cells, scrollback and grid
// wrap bits equal the model's and a second Emit writes nothing; the scrollback's wrap bits equal the
// model's until a line spans more rows than the viewport, which a batch must cut.
func TestConformance_RandomizedWrapWalk(t *testing.T) {
	steps, wrappedChecks := 0, 0
	for seed := int64(1); seed <= 400; seed++ {
		n, w := walkWraps(t, seed)
		steps, wrappedChecks = steps+n, wrappedChecks+w
	}
	require.NotZero(t, wrappedChecks, "test setup: the walk compares wrap bits while lines wrap")
	t.Logf("%d steps, %d wrap-bit comparisons with a wrapped line", steps, wrappedChecks)
}

// walkWraps runs one seed's 60 steps and returns them with how many compared a set wrap bit.
func walkWraps(
	t *testing.T,
	seed int64,
) (steps, wrappedChecks int) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	cols, rows := 5+rng.Intn(10), 3+rng.Intn(5)
	m, ser := New(cols, rows, 1000)
	defer m.Close()
	sim := newClientSim(t, cols, rows, 1000)
	defer func() { sim.m.Close() }()
	e := NewDiffEmitter()
	var chunks []string
	exactHistory := true
	for ; steps < 60; steps++ {
		chunks = append(chunks, wrapWalkChunk(rng, cols, rows))
		where := fmt.Sprintf("seed %d at %dx%d after %q", seed, cols, rows, chunks)
		m.Write([]byte(chunks[steps]))
		data, needKeyframe := e.Emit(m)
		if needKeyframe {
			sim.apply(ser.Serialize(m), true)
			e.Prime(m)
		} else if len(data) > 0 {
			sim.apply(data, false)
		}
		require.Equal(t, gridString(m), gridString(sim.m), where)
		require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m), where)
		again, needKeyframe := e.Emit(m)
		require.False(t, needKeyframe, where)
		require.Empty(t, string(again), "a second Emit rewrites nothing: %s", where)
		exactHistory = exactHistory && longestLine(t, m) <= rows
		want, got := wrapFlags(t, m), wrapFlags(t, sim.m)
		if !exactHistory {
			want, _, _ = strings.Cut(want, " | ")
			got, _, _ = strings.Cut(got, " | ")
		}
		require.Equal(t, want, got, where)
		if strings.Contains(want, "T") {
			wrappedChecks++
		}
	}
	return steps, wrappedChecks
}

// wrapWalkChunk returns one step: mostly text long enough to wrap, else a line break, an edit or
// erase anywhere, an autowrap flip, a line insert or delete, a scroll, a scroll region, or a burst.
func wrapWalkChunk(
	rng *rand.Rand,
	cols int,
	rows int,
) string {
	pick := func(s ...string) string { return s[rng.Intn(len(s))] }
	row := fmt.Sprintf("\x1b[%d;1H", 1+rng.Intn(rows))
	switch rng.Intn(16) {
	case 0, 1, 2, 3:
		return walkText(rng, cols*(rows-2))
	case 4, 5:
		return "\r\n" + walkText(rng, cols*(rows-2))
	case 6:
		return "\r\n"
	case 7, 8:
		return fmt.Sprintf("\x1b[%d;%dH", 1+rng.Intn(rows), 1+rng.Intn(cols)) + walkText(rng, cols)
	case 9:
		return fmt.Sprintf("\x1b[%d;%dH", rows, 1+rng.Intn(cols)) + walkText(rng, 2*cols)
	case 10:
		return pick("\x1b[K", "\x1b[1K", "\x1b[2K", row+"\x1b[K", row+"\x1b[2K", row+"\x1b[J")
	case 11:
		return pick("\x1b[?7l", "\x1b[?7h")
	case 12:
		return row + pick("\x1b[L", "\x1b[M", "\x1b[2L", "\x1b[2M")
	case 13:
		return pick("\x1b[S", "\x1b[T", "\x1bM")
	case 14:
		return pick(fmt.Sprintf("\x1b[%d;%dr", 1+rng.Intn(2), rows-1-rng.Intn(2)), "\x1b[r")
	default:
		return walkBurst(rng, cols, rows)
	}
}

// walkBurst commits several lines, each long enough to wrap, in one chunk.
func walkBurst(
	rng *rand.Rand,
	cols int,
	rows int,
) string {
	var b strings.Builder
	for n := 2 + rng.Intn(rows+2); n > 0; n-- {
		b.WriteString("\r\n" + walkText(rng, cols*(rows-2)))
	}
	return b.String()
}

// walkText returns up to n letters with an occasional space, so a head may end in a printed blank.
func walkText(
	rng *rand.Rand,
	n int,
) string {
	text := make([]byte, 1+rng.Intn(n))
	for i := range text {
		text[i] = "abcdefghijklmnopqrstuvwxyz        "[rng.Intn(30)]
	}
	return string(text)
}

// longestLine returns the most rows one line spans, across the scrollback and the grid.
func longestLine(
	t *testing.T,
	m TerminalModel,
) int {
	t.Helper()
	emu := asVT(t, m).emu
	sb := emu.ScrollbackLen()
	longest, run := 0, 0
	for i := 0; i < sb+emu.Height(); i++ {
		wrapped := i < sb && emu.ScrollbackRowWrapped(i) || i >= sb && emu.RowWrapped(i-sb)
		if !wrapped {
			run = 0
		}
		run++
		longest = max(longest, run)
	}
	return longest
}
