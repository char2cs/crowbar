package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
)

// resizeStep resizes the model and its client, sends the keyframe a resize demands and asserts the
// client rebuilt the model: grid, scrollback, wrap bits and cells.
func resizeStep(
	t *testing.T,
	m TerminalModel,
	e *DiffEmitter,
	ser Serializer,
	sim *clientSim,
	cols, rows int,
) {
	t.Helper()
	m.Resize(cols, rows)
	sim.resize(cols, rows)
	_, needKeyframe := e.Emit(m)
	require.True(t, needKeyframe, "a resize to %dx%d demands a keyframe", cols, rows)
	sim.apply(ser.Serialize(m), true)
	e.Prime(m)
	assertSound(t, m, "after the resize")
	require.Equal(t, gridString(m), gridString(sim.m), "grid at %dx%d", cols, rows)
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m), "scrollback at %dx%d", cols, rows)
	require.Equal(t, wrapFlags(t, m), wrapFlags(t, sim.m), "wrap bits at %dx%d", cols, rows)
	assertSameCells(t, m, sim.m)
}

// assertSound checks what every resize must leave: no escaped panic, and no scrollback line wider
// than the screen.
func assertSound(
	t *testing.T,
	m TerminalModel,
	where string,
) {
	t.Helper()
	vm := asVT(t, m)
	require.False(t, vm.Degraded(), "the model degraded: %s", where)
	for i := 0; i < vm.emu.ScrollbackLen(); i++ {
		require.LessOrEqual(t, len(vm.emu.ScrollbackLine(i)), vm.emu.Width(), "scrollback line %d: %s", i, where)
	}
}

// logicalLines joins the scrollback and the grid into the lines their wrap bits make, trimmed.
func logicalLines(
	t *testing.T,
	m TerminalModel,
) []string {
	t.Helper()
	emu := asVT(t, m).emu
	var lines []string
	add := func(text string, wrapped bool) {
		if wrapped && len(lines) > 0 {
			lines[len(lines)-1] += text
			return
		}
		lines = append(lines, text)
	}
	for i := 0; i < emu.ScrollbackLen(); i++ {
		add(padText(emu.ScrollbackLine(i), emu.Width()), emu.ScrollbackRowWrapped(i))
	}
	for y := 0; y < emu.Height(); y++ {
		add(plainText(snapshotRowInto(emu, nil, emu.Width(), y)), emu.RowWrapped(y))
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

// padText renders a stored scrollback line padded to cols, as the row it was on the screen.
func padText(
	line []uv.Cell,
	cols int,
) string {
	return plainText(padToCols(line, cols))
}

// resizeChains holds a 3-row chain, short lines and two more chains, and ends on a fresh line.
var resizeChains = "see " + wrapTestURL + "\r\nab\r\n" + strings.Repeat("a", 45) + "\r\nshort\r\n" +
	strings.Repeat("b", 30) + "\r\n"

func TestConformance_ResizeKeyframeRebuildsReflowedChains(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)
	conformanceStep(t, m, e, ser, sim, []byte("\r\n"+resizeChains+resizeChains))
	_, history, _ := strings.Cut(wrapFlags(t, m), "| ")
	require.Contains(t, history, "T", "test setup: a chain in the scrollback")

	for _, size := range [][2]int{{13, 6}, {20, 6}, {33, 6}, {20, 6}, {20, 4}, {20, 9}, {20, 6}} {
		resizeStep(t, m, e, ser, sim, size[0], size[1])
		require.False(t, conformanceStep(t, m, e, ser, sim, []byte(strings.Repeat("c", 25)+"\r\n")),
			"output after the %v keyframe takes the diff path", size)
	}
}

func TestConformance_WidthToggleRestoresTheModel(t *testing.T) {
	bytes := []byte(strings.Repeat("line\r\n", 8) + resizeChains + resizeChains)
	for _, cols := range []int{13, 7, 33} {
		ref, _ := newTestModel(t, 20, 6)
		ref.Write(bytes)
		m, _ := newTestModel(t, 20, 6)
		m.Write(bytes)

		m.Resize(cols, 6)
		m.Resize(20, 6)
		assertSound(t, m, "after the toggle")
		require.Equal(t, gridString(ref), gridString(m), "grid after 20 -> %d -> 20", cols)
		require.Equal(t, scrollbackStrings(ref), scrollbackStrings(m), "scrollback after 20 -> %d -> 20", cols)
		require.Equal(t, wrapFlags(t, ref), wrapFlags(t, m), "wrap bits after 20 -> %d -> 20", cols)
		assertSameCells(t, ref, m)
	}
}

func TestConformance_TallChainSurvivesResize(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)
	chain := strings.Repeat("x", 400)
	conformanceStep(t, m, e, ser, sim, []byte("\r\n"+chain+"\r\n"))

	for _, cols := range []int{7, 33, 20} {
		resizeStep(t, m, e, ser, sim, cols, 6)
		require.Contains(t, logicalLines(t, sim.m), chain, "the client holds the chain whole at %d columns", cols)
	}
}

func TestConformance_WideGlyphAtTheEdgeAcrossResize(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)
	wide := strings.Repeat("ab日", 12) + "\r\n" + strings.Repeat("x", 19) + "日本語\r\n" +
		strings.Repeat("y", 18) + "日z\r\n" + strings.Repeat("語", 15) + "\r\n"
	conformanceStep(t, m, e, ser, sim, []byte("\r\n"+wide+wide))

	for _, cols := range []int{13, 20, 7, 33, 19, 20} {
		resizeStep(t, m, e, ser, sim, cols, 6)
	}
}

func TestConformance_AltScreenIsNotReflowed(t *testing.T) {
	m, ser, sim, e := newWrapConformance(t)
	conformanceStep(t, m, e, ser, sim, []byte("\r\n"+resizeChains))
	conformanceStep(t, m, e, ser, sim, []byte("\x1b[?1049h\x1b[H0123456789ABCDEFGHIJKLMNO\r\nplain"))
	require.Equal(t, "grid F T F F F F", strings.SplitN(wrapFlags(t, m), " |", 2)[0], "test setup")

	m.Resize(12, 6)
	sim.resize(12, 6)
	_, needKeyframe := e.Emit(m)
	require.True(t, needKeyframe)
	sim.apply(ser.Serialize(m), true)
	e.Prime(m)
	require.Equal(t, "0123456789AB\nKLMNO\nplain\n\n\n", asVT(t, m).ScreenText(), "the alt grid is truncated")
	require.Equal(t, "grid F T F F F F", strings.SplitN(wrapFlags(t, m), " |", 2)[0], "and keeps its flags")
	require.Equal(t, gridString(m), gridString(sim.m))

	require.True(t, conformanceStep(t, m, e, ser, sim, []byte("\x1b[?1049l")), "leaving the alt screen")
	require.Equal(t, scrollbackStrings(m), scrollbackStrings(sim.m))
	require.Equal(t, wrapFlags(t, m), wrapFlags(t, sim.m))
	require.Contains(t, logicalLines(t, m), "see "+wrapTestURL, "the primary screen underneath was reflowed whole")
	assertSameCells(t, m, sim.m)
}

func TestConformance_RandomizedResizeWalk(t *testing.T) {
	steps, wrappedChecks := 0, 0
	for seed := int64(1); seed <= 400; seed++ {
		n, w := walkWraps(t, seed, true)
		steps, wrappedChecks = steps+n, wrappedChecks+w
	}
	require.NotZero(t, wrappedChecks, "test setup: the walk compares wrap bits while lines wrap")
	t.Logf("%d steps, %d wrap-bit comparisons with a wrapped line", steps, wrappedChecks)
}

// The web resize tests replay this fixture through xterm.js: a client fed before and resized must
// equal one fed after. After an intended change: go test -tags noEmbed . -run XtermResizeFixture -update
const xtermResizeFixturePath = "testdata/xterm-resize-reflow.json"

type xtermResizeScenario struct {
	Name        string `json:"name"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
	Scrollback  int    `json:"scrollback"`
	ToCols      int    `json:"toCols"`
	ToRows      int    `json:"toRows"`
	Raw         string `json:"raw"`
	Before      string `json:"before"`
	After       string `json:"after"`
	Saturated   bool   `json:"saturated"`   // the cap trimmed the oldest line, whose wrap bit no keyframe carries
	PendingWrap bool   `json:"pendingWrap"` // the cursor sat at a pending wrap, which a keyframe's CUP cannot carry
}

func TestConformance_XtermResizeFixture(t *testing.T) {
	var history strings.Builder
	for i := 0; i < 30; i++ {
		history.WriteString(strings.Repeat(string(rune('a'+i%26)), 45) + "\r\n")
	}
	alt := resizeChains + "\x1b[?1049h\x1b[H" + strings.Repeat("A", 25) + "\r\nalt"
	wide := strings.Repeat("ab日", 10) + "\r\n" + strings.Repeat("x", 19) + "日本語\r\n"
	scenarios := []xtermResizeScenario{
		{Name: "shrink", ToCols: 13, ToRows: 6, Raw: resizeChains},
		{Name: "grow", ToCols: 33, ToRows: 6, Raw: resizeChains},
		{Name: "chainInHistoryShrink", ToCols: 13, ToRows: 6, Raw: resizeChains + strings.Repeat("\r\n", 6)},
		{Name: "chainInHistoryGrow", ToCols: 33, ToRows: 6, Raw: resizeChains + strings.Repeat("\r\n", 6)},
		{Name: "wideAtTheEdge", ToCols: 13, ToRows: 6, Raw: wide + wide},
		{Name: "cursorChain", ToCols: 13, ToRows: 6, Raw: "short\r\n" + strings.Repeat("z", 45)},
		{Name: "heightShrink", ToCols: 20, ToRows: 4, Raw: resizeChains},
		{Name: "heightGrow", ToCols: 20, ToRows: 9, Raw: resizeChains + strings.Repeat("\r\n", 6)},
		{Name: "altScreen", ToCols: 13, ToRows: 6, Raw: alt},
		{Name: "saturated", ToCols: 13, ToRows: 6, Raw: history.String(), Saturated: true},
		{Name: "pendingWrap", ToCols: 33, ToRows: 6, Raw: "\r\n" + strings.Repeat("p", 20), PendingWrap: true},
		// Known divergence (UPSTREAM.md D2): x/vt cannot tell the printed spaces ending this chain
		// from erased cells, so the daemon counts 20 cells where xterm counts 23.
		{Name: "printedSpacesKnownDivergence", ToCols: 10, ToRows: 6, Raw: strings.Repeat("x", 20) + "   \r\nnext"},
	}
	for i := range scenarios {
		sc := &scenarios[i]
		sc.Cols, sc.Rows, sc.Scrollback = 20, 6, 50
		m, ser := New(sc.Cols, sc.Rows, sc.Scrollback)
		m.Write([]byte(sc.Raw))
		sc.Before = string(ser.Serialize(m))
		m.Resize(sc.ToCols, sc.ToRows)
		sc.After = string(ser.Serialize(m))
		m.Close()
	}
	encoded, err := json.MarshalIndent(struct {
		Scenarios []xtermResizeScenario `json:"scenarios"`
	}{scenarios}, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')
	if *updateXtermFixture {
		require.NoError(t, os.WriteFile(xtermResizeFixturePath, encoded, 0o600))
	}
	want, err := os.ReadFile(xtermResizeFixturePath)
	require.NoError(t, err)
	require.Equal(t, string(want), string(encoded), "the resize changed: rerun with -update, then the web resize tests")
}
