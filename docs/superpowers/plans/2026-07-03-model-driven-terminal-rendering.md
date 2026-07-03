# Model-Driven Terminal Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the daemon's VT screen model the sole source of the bytes terminal clients receive: grid-diff frames on an adaptive 8ms clock, keyframes via the existing snapshot frames, per-session flag with raw-path fallback — so client state ≡ model state by construction.

**Architecture:** A new `DiffEmitter` in the `model` package (paired with `vtModel` behind the same package wall as the serializer) computes scrollback-delta + dirty-line + chrome-delta ANSI between emits. The session, when the flag is on, reorders its pump to model-write → scheduled emit (instead of raw fan-out → model-write), emits keyframes (existing serializer, `Snapshot:true` frames from PR #26) on invalidation, forwards the model's device-query answers to the PTY, and falls back to raw streaming when the model is degraded.

**Tech Stack:** Go (x/vt pinned `v0.0.0-20260629091435-9c70f75e26a4`, ultraviolet, x/ansi), existing terminal engine. No frontend or wire changes beyond what PR #26 shipped.

**Spec:** `docs/superpowers/specs/2026-07-03-model-driven-terminal-rendering-design.md`

## Global Constraints

- Do NOT change the pinned x/vt / ultraviolet / x/ansi module versions (model/UPSTREAM.md).
- The raw-stream path must remain fully functional and untouched with the flag OFF; every existing test must pass unchanged.
- Flag: env `CROWBAR_TERMINAL_MODEL_DRIVEN` ("1"/"true" → on, "0"/"false" → off) overrides the build default (ON under the `noEmbed` build tag = dev daemon, OFF otherwise).
- Frame clock constant: `minEmitInterval = 8 * time.Millisecond`.
- All model-package types are NOT goroutine-safe; the session serialises access under `s.mu` (existing rule).
- Only touch `api/internal/engine/terminal/...` and the spec/plan docs — another session owns in-progress work under `api/internal/api/v0/endpoints/workspaces/` and `api/internal/app/`; never `git add -A`, always add explicit paths.
- Run tests as: `cd api && go test -tags noEmbed -race ./internal/engine/terminal/...`
- gofumpt any file you touch: `go run mvdan.cc/gofumpt@latest -w <files>` (note: `model/shadow_state.go`, `model/vt_model.go`, `session_panic_test.go`, `terminal_hardening_test.go` have PRE-EXISTING drift — do not reformat files you don't otherwise modify).

---

### Task 1: DiffEmitter — screen-line diff core

**Files:**
- Create: `api/internal/engine/terminal/internal/model/diff.go`
- Test: `api/internal/engine/terminal/internal/model/diff_test.go`

**Interfaces:**
- Consumes: `TerminalModel` (model.go), `*vtModel` internals (`vm.emu` `emulator`: `Width() int`, `Height() int`, `CellAt(x,y) *uv.Cell`, `CursorPosition() uv.Position`, `IsAltScreen() bool`, `ScrollbackLen() int`, `ScrollbackLine(y) uv.Line`), `encodeGridRow(emu emulator, cols, y int) string` (vt_serializer.go), `uv "github.com/charmbracelet/ultraviolet"`, `"github.com/charmbracelet/x/ansi"`.
- Produces (later tasks rely on these exact signatures):
  - `func NewDiffEmitter() *DiffEmitter`
  - `func (e *DiffEmitter) Emit(m TerminalModel) (data []byte, needKeyframe bool)`
  - `func (e *DiffEmitter) Prime(m TerminalModel)`
  - `func (e *DiffEmitter) Invalidate()`

- [ ] **Step 1: Write the failing tests**

`diff_test.go` (new file, `package model`). Use the same construction helpers existing model tests use — check `vt_model_test.go` for the canonical way to build a model (`New("id", cols, rows, scrollback)` or equivalent; mirror it exactly):

```go
package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestModel mirrors vt_model_test.go's construction (adjust to the actual
// factory used there — the model and serializer are built together by New).
func newTestModel(t *testing.T, cols, rows int) (TerminalModel, Serializer) {
	t.Helper()
	m, s := New(cols, rows, 200) // ADAPT: copy the exact call from vt_model_test.go
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
	_ = strings.TrimSpace("")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test -tags noEmbed -run TestDiffEmitter ./internal/engine/terminal/internal/model/ -v`
Expected: FAIL — `undefined: NewDiffEmitter` (fix the `newTestModel` factory call against the real one from vt_model_test.go first if it doesn't compile).

- [ ] **Step 3: Implement the screen-diff core**

`diff.go` (new file):

```go
package model

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// DiffEmitter converts model state changes into minimal incremental ANSI for
// live clients (spec §3.2). It is the streaming counterpart of the Serializer:
// where Serialize emits one full ground-state redraw, Emit produces only what
// changed since the previous Emit/Prime. Like the Serializer it reads the
// concrete model through the package wall (type assertion), never through the
// session-facing interface. Not goroutine-safe; the owning session serialises
// calls under its lock.
//
// Lifecycle: an unprimed (or invalidated) emitter answers Emit with
// needKeyframe=true; the caller then sends a full snapshot (Serializer) and
// calls Prime to re-base. Dimension changes, alt-screen flips and scrollback
// shrinkage (ED3-style clears) are detected in Emit and demand a keyframe the
// same way — a client reset+redraw is both simpler and cheaper than a
// worst-case whole-screen diff with epoch bookkeeping.
type DiffEmitter struct {
	valid bool

	cols, rows    int
	alt           bool
	scrollbackLen int
	lastGrid      [][]uv.Cell
	lastCursor    uv.Position
}

// NewDiffEmitter returns an unprimed emitter (first Emit demands a keyframe).
func NewDiffEmitter() *DiffEmitter {
	return &DiffEmitter{}
}

// Invalidate discards the diff base; the next Emit demands a keyframe.
func (e *DiffEmitter) Invalidate() {
	e.valid = false
}

// Prime captures the model's current grid/cursor/counters as the diff base.
// Call immediately after emitting a keyframe serialized from the same locked
// model state.
func (e *DiffEmitter) Prime(m TerminalModel) {
	vm := m.(*vtModel)
	e.cols, e.rows = vm.emu.Width(), vm.emu.Height()
	e.alt = vm.emu.IsAltScreen()
	e.scrollbackLen = vm.emu.ScrollbackLen()
	e.lastCursor = vm.emu.CursorPosition()
	e.lastGrid = snapshotGrid(vm.emu, e.cols, e.rows)
	e.valid = true
}

// Emit returns the incremental ANSI since the last Prime/Emit, or
// needKeyframe=true when diffing is impossible (unprimed, invalidated, resize,
// alt-screen flip, scrollback shrink). On success the emitter re-bases itself.
func (e *DiffEmitter) Emit(m TerminalModel) (data []byte, needKeyframe bool) {
	vm := m.(*vtModel)
	cols, rows := vm.emu.Width(), vm.emu.Height()
	alt := vm.emu.IsAltScreen()
	sbLen := vm.emu.ScrollbackLen()

	if !e.valid || cols != e.cols || rows != e.rows || alt != e.alt || sbLen < e.scrollbackLen {
		return nil, true
	}

	var b strings.Builder
	e.writeScrollbackDelta(&b, vm, sbLen, rows)
	dirty := e.writeScreenDiff(&b, vm, cols, rows)
	e.writeCursorDelta(&b, vm, dirty)

	e.scrollbackLen = sbLen
	e.lastCursor = vm.emu.CursorPosition()
	if b.Len() == 0 {
		return nil, false
	}
	return []byte(b.String()), false
}

// writeScreenDiff rewrites every changed grid row in place (CUP + encoded row,
// pen reset per row via encodeLine's contract) and updates the diff base.
// Returns whether anything was written.
func (e *DiffEmitter) writeScreenDiff(
	b *strings.Builder,
	vm *vtModel,
	cols int,
	rows int,
) bool {
	dirty := false
	for y := 0; y < rows; y++ {
		row := snapshotRow(vm.emu, cols, y)
		if rowsEqual(e.lastGrid[y], row) {
			continue
		}
		dirty = true
		b.WriteString(ansi.CursorPosition(1, y+1)) // (col, row), 1-based
		b.WriteString(ansi.EraseLineRight)
		b.WriteString(encodeGridRow(vm.emu, cols, y))
		e.lastGrid[y] = row
	}
	return dirty
}

// writeCursorDelta repositions the client cursor to the model's position when
// it moved or when screen rewrites displaced it.
func (e *DiffEmitter) writeCursorDelta(
	b *strings.Builder,
	vm *vtModel,
	dirty bool,
) {
	cur := vm.emu.CursorPosition()
	if !dirty && cur == e.lastCursor {
		return
	}
	b.WriteString(ansi.CursorPosition(cur.X+1, cur.Y+1))
}

func snapshotGrid(emu emulator, cols, rows int) [][]uv.Cell {
	grid := make([][]uv.Cell, rows)
	for y := 0; y < rows; y++ {
		grid[y] = snapshotRow(emu, cols, y)
	}
	return grid
}

func snapshotRow(emu emulator, cols, y int) []uv.Cell {
	row := make([]uv.Cell, cols)
	for x := 0; x < cols; x++ {
		if c := emu.CellAt(x, y); c != nil {
			row[x] = *c
		} else {
			row[x] = uv.EmptyCell
		}
	}
	return row
}

func rowsEqual(a, b []uv.Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(&b[i]) {
			return false
		}
	}
	return true
}
```

NOTE for the implementer: `writeScrollbackDelta` is Task 2 — for this task add the stub `func (e *DiffEmitter) writeScrollbackDelta(*strings.Builder, *vtModel, int, int) {}` with a `// Task 2` comment so this task compiles and its tests pass. Verify `ansi.CursorPosition`'s argument order against x/ansi (`go doc github.com/charmbracelet/x/ansi CursorPosition`) — if it is (row, col), swap the arguments here and in the tests. Verify `uv.Cell.Equal` exists (`go doc github.com/charmbracelet/ultraviolet Cell`); if not, compare with `reflect.DeepEqual` on the row slices (measured later in Task 5's bench — replace only if it shows).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed -race -run TestDiffEmitter ./internal/engine/terminal/internal/model/ -v`
Expected: PASS (all 7).

- [ ] **Step 5: Record the ultraviolet-spike decision (spec P0)**

Append to `api/internal/engine/terminal/internal/model/UPSTREAM.md`:

```markdown
## Diff emitter backend (2026-07-03, model-driven rendering P0)

Hand-rolled line diff adopted (diff.go) over ultraviolet's renderer:
uv's diff/render pipeline is a TUI *presenter* (owns cursor state, assumes it
is the only writer, renders to an io.Writer with its own frame lifecycle);
constraining its output to our emit contract (absolute CUP per dirty row, pen
reset per row via encodeLine, no cursor-state ownership) would mean fighting
the library. The hand-rolled diff is ~120 lines and reuses the serializer's
encodeGridRow/encodeLine, so both emission paths share one cell renderer.
```

- [ ] **Step 6: Commit**

```bash
cd <worktree-root>
git add api/internal/engine/terminal/internal/model/diff.go api/internal/engine/terminal/internal/model/diff_test.go api/internal/engine/terminal/internal/model/UPSTREAM.md
git commit -m "feat(terminal): DiffEmitter screen-line diff core (model-driven rendering P1)"
```

---

### Task 2: DiffEmitter — scrollback delta

**Files:**
- Modify: `api/internal/engine/terminal/internal/model/diff.go` (replace the Task 1 stub)
- Test: `api/internal/engine/terminal/internal/model/diff_test.go` (append)

**Interfaces:**
- Consumes: `vm.emu.ScrollbackLen()`, `vm.emu.ScrollbackLine(y) uv.Line`, `encodeLine(cells []uv.Cell, width int, trim bool) string` (vt_serializer.go), `ansi.CursorPosition`.
- Produces: `writeScrollbackDelta` emitting committed-since-last-emit scrollback lines BEFORE the screen diff, so they scroll into the client's own scrollback (same technique as the serializer's writeContent flow).

- [ ] **Step 1: Write the failing tests**

Append to `diff_test.go`:

```go
func TestDiffEmitter_ScrollbackDeltaEmittedBeforeScreenDiff(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)

	// Write 5 numbered lines into a 3-row screen: 2+ lines commit to scrollback.
	m.Write([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive"))
	data, need := e.Emit(m)
	require.False(t, need)
	s := string(data)

	// Committed scrollback lines are emitted as bottom-row writes + newline
	// so the CLIENT scrolls them into its own history.
	assert.Contains(t, s, "one")
	assert.Contains(t, s, "two")
	// The scrollback flow must precede the first screen-diff CUP.
	sbIdx := strings.Index(s, "one")
	screenIdx := strings.Index(s, "\x1b[1;1H")
	require.GreaterOrEqual(t, screenIdx, 0)
	assert.Less(t, sbIdx, screenIdx, "scrollback delta must precede screen diff")
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test -tags noEmbed -run TestDiffEmitter_Scrollback ./internal/engine/terminal/internal/model/ -v`
Expected: FAIL (scrollback lines missing from output / ordering assertion fails).

- [ ] **Step 3: Implement writeScrollbackDelta**

Replace the Task 1 stub in `diff.go`:

```go
// writeScrollbackDelta emits every scrollback line the model committed since
// the last emit. Technique (mirrors the serializer's writeContent flow): park
// the cursor on the bottom row, then write each line followed by CR+LF — the
// client scrolls, the line enters ITS scrollback, and the screen area is
// repainted by the screen diff that follows (which sees all rows dirty after
// a scroll anyway). Primary buffer only; the alt screen has no scrollback.
func (e *DiffEmitter) writeScrollbackDelta(
	b *strings.Builder,
	vm *vtModel,
	sbLen int,
	rows int,
) {
	if e.alt || sbLen <= e.scrollbackLen {
		return
	}
	b.WriteString(ansi.CursorPosition(1, rows)) // park on the bottom row
	for y := e.scrollbackLen; y < sbLen; y++ {
		line := vm.emu.ScrollbackLine(y)
		b.WriteString("\r\n")
		b.WriteString(encodeLine(line, len(line), true))
	}
}
```

Then force the screen diff to repaint everything after a scroll: in `Emit`, when `sbLen > e.scrollbackLen`, invalidate the row cache before diffing (the client's screen scrolled, so row identity moved):

```go
	// In Emit, after the keyframe guard and before writeScrollbackDelta:
	if sbLen > e.scrollbackLen {
		// The client screen scrolls while absorbing the delta; every row's
		// on-screen identity moves, so rebuild the whole viewport after.
		for y := range e.lastGrid {
			e.lastGrid[y] = nil // nil never equals a real row → forced rewrite
		}
	}
```

(`rowsEqual(nil, row)` is false because lengths differ — no extra code needed.)

- [ ] **Step 4: Run the whole DiffEmitter suite**

Run: `cd api && go test -tags noEmbed -race -run TestDiffEmitter ./internal/engine/terminal/internal/model/ -v`
Expected: PASS (all 10).

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/terminal/internal/model/diff.go api/internal/engine/terminal/internal/model/diff_test.go
git commit -m "feat(terminal): DiffEmitter scrollback delta (model-driven rendering P1)"
```

---

### Task 3: DiffEmitter — chrome delta (modes, title, colors, cursor style)

**Files:**
- Modify: `api/internal/engine/terminal/internal/model/diff.go`
- Test: `api/internal/engine/terminal/internal/model/diff_test.go` (append)

**Interfaces:**
- Consumes: `vm.shadow` (`shadowState`: `modes map[int]bool`, `title string`, `cursorVisible bool` / cursor-style and color fields — read the exact field names from `shadow_state.go` and the serializer's `writeModes`/`writeCursor`/`writeChrome` before coding), `serializedModeOrder`, `ansi.SetMode`/`ansi.ResetMode`.
- Produces: chrome changes stream to clients between keyframes — an app toggling mouse tracking / bracketed paste / title mid-session reaches the client under the flag.

- [ ] **Step 1: Write the failing tests**

Append to `diff_test.go` (adapt shadow field access to the real names found in `shadow_state.go`):

```go
func TestDiffEmitter_ModeToggleForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?2004h")) // bracketed paste ON
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b[?2004h")

	m.Write([]byte("\x1b[?2004l")) // OFF again
	data, _ = e.Emit(m)
	assert.Contains(t, string(data), "\x1b[?2004l")
}

func TestDiffEmitter_TitleChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b]0;my-title\x07"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b]0;my-title\x07")
}

func TestDiffEmitter_UnchangedChromeEmitsNothing(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	m.Write([]byte("\x1b[?2004h\x1b]0;t\x07"))
	e := NewDiffEmitter()
	e.Prime(m)
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Empty(t, data, "already-primed chrome must not re-emit")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test -tags noEmbed -run "TestDiffEmitter_(Mode|Title|Unchanged)" ./internal/engine/terminal/internal/model/ -v`
Expected: FAIL.

- [ ] **Step 3: Implement the chrome delta**

Add to `DiffEmitter` state and lifecycle (`Prime` captures, `Emit` diffs):

```go
// chromeBase is the subset of shadow state whose CHANGES must stream to live
// clients between keyframes. Grid content is covered by the screen diff; this
// covers everything else a client-side terminal tracks statefully.
type chromeBase struct {
	modes  map[int]bool
	title  string
	// ADAPT: add the shadow's cursor-visibility, cursor-style, keypad and
	// default-color fields here with the exact names from shadow_state.go,
	// mirroring what writeModes/writeCursor/writeChrome serialize.
}

func captureChrome(sh *shadowState) chromeBase {
	modes := make(map[int]bool, len(sh.modes))
	for k, v := range sh.modes {
		modes[k] = v
	}
	return chromeBase{modes: modes, title: sh.title}
}

// writeChromeDelta emits mode flips, title changes and cursor-chrome changes
// since the diff base. Mode order follows serializedModeOrder for determinism;
// modes outside that list still stream (sorted numerically) so nothing an app
// set is silently dropped.
func (e *DiffEmitter) writeChromeDelta(
	b *strings.Builder,
	sh *shadowState,
) {
	keys := make([]int, 0, len(sh.modes)+len(e.chrome.modes))
	seen := map[int]bool{}
	for _, k := range serializedModeOrder {
		keys = append(keys, k)
		seen[k] = true
	}
	extra := make([]int, 0)
	for k := range sh.modes {
		if !seen[k] {
			extra = append(extra, k)
			seen[k] = true
		}
	}
	for k := range e.chrome.modes {
		if !seen[k] {
			extra = append(extra, k)
			seen[k] = true
		}
	}
	sort.Ints(extra)
	keys = append(keys, extra...)

	for _, mode := range keys {
		now, nowOK := sh.modes[mode]
		was, wasOK := e.chrome.modes[mode]
		if nowOK == wasOK && now == was {
			continue
		}
		if now {
			b.WriteString(ansi.SetMode(ansi.DECMode(mode)))
		} else {
			b.WriteString(ansi.ResetMode(ansi.DECMode(mode)))
		}
	}
	if sh.title != e.chrome.title {
		b.WriteString("\x1b]0;" + sanitizeOSCText(sh.title) + "\x07")
	}
	// ADAPT: cursor visibility/style + default colors, mirroring the
	// serializer's writeCursor/writeChrome emission forms, diffed the same way.
}
```

Wire into `Emit` (after `writeCursorDelta`) and `Prime` (`e.chrome = captureChrome(&vm.shadow)`); after a successful Emit, re-capture: `e.chrome = captureChrome(&vm.shadow)`. `sanitizeOSCText`: reuse the serializer's existing OSC text sanitizer (grep `maxOSCTextRunes` usage in vt_serializer.go for the helper name; if it is inline, extract it to a shared func in that file). Add `"sort"` to imports.

- [ ] **Step 4: Run the full model package**

Run: `cd api && go test -tags noEmbed -race ./internal/engine/terminal/internal/model/ -v 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"`
Expected: `ok` (existing serializer tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/terminal/internal/model/diff.go api/internal/engine/terminal/internal/model/diff_test.go api/internal/engine/terminal/internal/model/vt_serializer.go
git commit -m "feat(terminal): DiffEmitter chrome delta — modes, title, cursor chrome (P1)"
```

---

### Task 4: Conformance property test (the centerpiece)

**Files:**
- Create: `api/internal/engine/terminal/internal/model/conformance_test.go`

**Interfaces:**
- Consumes: `New` (model factory), `DiffEmitter`, `Serializer.Serialize(m)`.
- Produces: the proof that emitted streams reconstruct model state exactly; every future emitter change must keep this green.

- [ ] **Step 1: Write the conformance test**

```go
package model

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// applyEmission drives one step of the client-simulator protocol: keyframes
// reset the client model (fresh instance) and apply the serialized redraw;
// diff frames apply incrementally.
type clientSim struct {
	t    *testing.T
	cols, rows, sb int
	m    TerminalModel
}

func newClientSim(t *testing.T, cols, rows, sb int) *clientSim {
	c := &clientSim{t: t, cols: cols, rows: rows, sb: sb}
	c.reset()
	return c
}

func (c *clientSim) reset() {
	if c.m != nil {
		c.m.Close()
	}
	m, _ := New(c.cols, c.rows, c.sb) // ADAPT to the real factory
	c.m = m
}

func (c *clientSim) apply(data []byte, keyframe bool) {
	if keyframe {
		c.reset()
	}
	c.m.Write(data)
}

// gridString renders the visible grid of a model as plain text for equality
// assertions (content-level conformance; SGR conformance is covered by the
// serializer's own tests since both paths share encodeLine).
func gridString(m TerminalModel) string {
	vm := m.(*vtModel)
	cols, rows := vm.emu.Width(), vm.emu.Height()
	out := ""
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if cell := vm.emu.CellAt(x, y); cell != nil {
				out += cell.String()
			} else {
				out += " "
			}
		}
		out += "\n"
	}
	cur := vm.emu.CursorPosition()
	return out + fmt.Sprintf("cursor=%d,%d alt=%v", cur.X, cur.Y, vm.emu.IsAltScreen())
}

// step feeds bytes to the authoritative model, emits, applies to the client
// sim, and asserts grid equality.
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
		{"a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng"},                       // scroll-through
		{"\x1b[?1049h", "TUI FRAME", "\x1b[2;3Hmid", "\x1b[?1049l"}, // alt round-trip
		{"\x1b[31mred\x1b[0m plain", "\x1b[1;44mbold-on-blue\x1b[m"},
		{"\x1b]0;title-a\x07out", "\x1b[?2004h", "paste-mode-on"},
		{"tab\thandling", "\rcarriage", "\x1b[5Dback"},
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
```

NOTE: mid-sequence splits mean the model buffers partial escapes (`PendingInput`). `conformanceStep` compares AFTER each chunk — a half-delivered escape leaves both models in the same visible state (authoritative model buffered it; the emitter saw no grid change; the sim never received it), so equality holds. If a scenario legitimately fails on pending-input edge cases, append the model's `PendingInput()` handling the way `Session.Attach` does — but ONLY after understanding the failure; do not blind-patch.

- [ ] **Step 2: Run it**

Run: `cd api && go test -tags noEmbed -race -run TestConformance ./internal/engine/terminal/internal/model/ -v`
Expected: PASS. If a scenario fails, THAT IS THE TEST DOING ITS JOB — fix the emitter (never weaken an assertion) before proceeding.

- [ ] **Step 3: Commit**

```bash
git add api/internal/engine/terminal/internal/model/conformance_test.go
git commit -m "test(terminal): conformance property suite — client sim ≡ model per frame (P1)"
```

---

### Task 5: Regime benchmarks

**Files:**
- Create: `api/internal/engine/terminal/internal/model/diff_bench_test.go`

**Interfaces:**
- Consumes: model factory, `DiffEmitter`.
- Produces: baseline numbers for the spec §4 budgets; run before AND after any emitter optimization.

- [ ] **Step 1: Write the benchmarks**

```go
package model

import (
	"fmt"
	"testing"
)

// BenchmarkDiffEcho: single-keystroke delta (the interactive hot path).
func BenchmarkDiffEcho(b *testing.B) {
	m, _ := New(170, 50, 1000) // ADAPT factory
	defer m.Close()
	e := NewDiffEmitter()
	e.Prime(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Write([]byte("x"))
		_, _ = e.Emit(m)
	}
}

// BenchmarkDiffFullRepaint: a TUI rewriting the whole screen per frame.
func BenchmarkDiffFullRepaint(b *testing.B) {
	m, _ := New(170, 50, 1000)
	defer m.Close()
	e := NewDiffEmitter()
	e.Prime(m)
	frameA := buildFullScreen('A', 170, 50)
	frameB := buildFullScreen('B', 170, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			m.Write(frameA)
		} else {
			m.Write(frameB)
		}
		_, _ = e.Emit(m)
	}
}

// BenchmarkDiffScrollBurst: cat-style append of 100 lines per emit tick.
func BenchmarkDiffScrollBurst(b *testing.B) {
	m, _ := New(170, 50, 10000)
	defer m.Close()
	e := NewDiffEmitter()
	e.Prime(m)
	var burst []byte
	for i := 0; i < 100; i++ {
		burst = append(burst, []byte(fmt.Sprintf("line %04d with some payload text\r\n", i))...)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Write(burst)
		_, _ = e.Emit(m)
	}
}

func buildFullScreen(ch byte, cols, rows int) []byte {
	out := []byte("\x1b[1;1H")
	line := make([]byte, cols)
	for i := range line {
		line[i] = ch
	}
	for y := 0; y < rows; y++ {
		out = append(out, []byte(fmt.Sprintf("\x1b[%d;1H", y+1))...)
		out = append(out, line...)
	}
	return out
}
```

- [ ] **Step 2: Run and record**

Run: `cd api && go test -tags noEmbed -bench BenchmarkDiff -benchmem -run '^$' ./internal/engine/terminal/internal/model/ | tee /tmp/diff-bench-baseline.txt`
Expected: all three complete. Budgets (spec §4): Echo < 100µs/op; FullRepaint and ScrollBurst < 8ms/op (one frame tick). If a budget is missed, profile before optimizing (`-cpuprofile`); the likely first win is replacing per-cell `CellAt` copying with row-at-a-time access if the emulator offers one.

- [ ] **Step 3: Commit**

```bash
git add api/internal/engine/terminal/internal/model/diff_bench_test.go
git commit -m "bench(terminal): diff emitter regime benchmarks (P0 baselines)"
```

---

### Task 6: Session integration — flag, pump reorder, degraded fallback

**Files:**
- Create: `api/internal/engine/terminal/modeldriven_dev.go`, `api/internal/engine/terminal/modeldriven_release.go`
- Modify: `api/internal/engine/terminal/internal/session/session.go` (spawn params, pumpStep), `api/internal/engine/terminal/terminal.go` (engineBirth plumb)
- Test: `api/internal/engine/terminal/internal/session/session_modeldriven_test.go` (new), `api/internal/engine/terminal/modeldriven_flag_test.go` (new)

**Interfaces:**
- Consumes: `model.NewDiffEmitter`, `s.serializeLocked()`, `s.fanOutLocked`, `OutputFrame{Snapshot: true}` (PR #26), `s.modelPanics`.
- Produces:
  - session field `emitter *model.DiffEmitter`, `modelDriven bool`
  - `session.New(id, shell, cwd, profileID string, env []string, cols, rows, scrollbackLines int)` UNCHANGED; instead `spawnParams`/`engineBirth` gain `ModelDriven bool` and `NewPlaceholder` is unchanged (restore reads the engine default at restore time)
  - engine: `func modelDrivenEnabled() bool` (env override + build default)
  - `func (s *Session) emitFrameLocked()` — Task 7 wires the clock; THIS task calls it synchronously per pumpStep (correct, unoptimized)

- [ ] **Step 1: Write the failing flag test**

`api/internal/engine/terminal/modeldriven_flag_test.go`:

```go
package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelDrivenEnabled_EnvOverridesDefault(t *testing.T) {
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "0")
	assert.False(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "false")
	assert.False(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "1")
	assert.True(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "true")
	assert.True(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "")
	assert.Equal(t, modelDrivenBuildDefault, modelDrivenEnabled())
}
```

- [ ] **Step 2: Implement the flag**

`modeldriven_dev.go`:

```go
//go:build noEmbed

package terminal

// modelDrivenBuildDefault: dev daemons (noEmbed) default the model-driven
// output path ON so it bakes under daily use (spec §3.7).
const modelDrivenBuildDefault = true
```

`modeldriven_release.go`:

```go
//go:build !noEmbed

package terminal

// modelDrivenBuildDefault: release builds keep the raw path until the dev
// bake period and the divergence canary pass (spec §3.7).
const modelDrivenBuildDefault = false
```

In `terminal.go` add:

```go
// modelDrivenEnabled resolves the model-driven output flag: the
// CROWBAR_TERMINAL_MODEL_DRIVEN env var when set ("1"/"true" on, "0"/"false"
// off), otherwise the build default. Read at session spawn/restore; never
// mid-session (spec §7).
func modelDrivenEnabled() bool {
	switch os.Getenv("CROWBAR_TERMINAL_MODEL_DRIVEN") {
	case "1", "true":
		return true
	case "0", "false":
		return false
	}
	return modelDrivenBuildDefault
}
```

Run: `cd api && go test -tags noEmbed -run TestModelDrivenEnabled ./internal/engine/terminal/ -v` → PASS.

- [ ] **Step 3: Write the failing session test**

`session_modeldriven_test.go` (`package session`, `//go:build !windows`):

```go
//go:build !windows

package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collect drains frames for d, concatenating data and remembering snapshots.
func collect(ch <-chan OutputFrame, d time.Duration) (data string, snapshots int) {
	deadline := time.After(d)
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return data, snapshots
			}
			if f.Snapshot {
				snapshots++
			}
			data += string(f.Data)
		case <-deadline:
			return data, snapshots
		}
	}
}

func TestModelDriven_OutputIsModelDerived(t *testing.T) {
	s, err := NewModelDriven("sid-md", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("echo MD-MARKER-42\n")))
	data, _ := collect(ch, 3*time.Second)
	// The marker must arrive (via diff frames), proving the emit path works
	// end to end. Diff frames use absolute cursor addressing, which raw shell
	// echo never emits at the prompt for plain output.
	assert.Contains(t, data, "MD-MARKER-42")
	assert.Contains(t, data, "\x1b[", "model-driven output is synthesized ANSI")
}

func TestModelDriven_DegradedFallsBackToRaw(t *testing.T) {
	s, err := NewModelDriven("sid-md-deg", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Force the degraded state the way the panic tests do (see
	// session_panic_test.go / session_testseams.go for the seam that swaps in
	// a panicking model — reuse it exactly).
	forceModelPanicForTest(s)

	require.NoError(t, s.Write([]byte("echo RAW-FALLBACK-7\n")))
	data, _ := collect(ch, 3*time.Second)
	assert.Contains(t, data, "RAW-FALLBACK-7", "degraded session must still stream raw")
	_ = strings.TrimSpace("")
}
```

`NewModelDriven` is a thin test-facing constructor added in the next step (mirrors how other test entry points live in `export_test.go` / `session_testseams.go` — put it where those live if unexported access is needed). `forceModelPanicForTest`: reuse/extract the existing panic-seam from `session_panic_test.go`; if none is directly callable, add one to `session_testseams.go` that swaps `s.model` for the existing panicking test model under `s.mu`.

- [ ] **Step 4: Implement session integration**

In `session.go`:

1. Fields on `Session`:

```go
	// Model-driven output (spec 2026-07-03): when set, clients receive
	// model-derived diff/keyframe frames instead of raw PTY bytes. Falls back
	// to raw streaming for the session's lifetime once the model degrades
	// (modelPanics > 0). emitter state is guarded by s.mu like the model.
	modelDriven bool
	emitter     *model.DiffEmitter
```

2. Construction: `spawnParams` (and `engineBirth` in terminal.go) gain `ModelDriven bool`; `spawn` sets `s.modelDriven = p.ModelDriven; if p.ModelDriven { s.emitter = model.NewDiffEmitter() }`. Engine `Create` and `restore` pass `ModelDriven: modelDrivenEnabled()`. Add the test constructor in `session_testseams.go`:

```go
// NewModelDriven is the test-facing constructor for a model-driven session.
func NewModelDriven(id, shell, cwd, profileID string, env []string, cols, rows, scrollback int) (*Session, error) {
	s := newBareSession(id, shell, cwd, profileID)
	if err := s.spawn(env, spawnParams{Cols: cols, Rows: rows, ScrollbackLines: scrollback, ModelDriven: true}); err != nil {
		return nil, err
	}
	return s, nil
}
```

(ADAPT: mirror the real `New` body exactly — copy it and add the flag.)

3. Pump reorder in `pumpStep`:

```go
func (s *Session) pumpStep(chunk []byte) {
	path, ok := parseLastOSC7(chunk)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.cwd = path
	}
	if s.useModelDrivenLocked() {
		// Model-driven (spec §3.1): the model is written FIRST and clients
		// receive model-derived frames. Raw fan-out is skipped entirely.
		s.writeModelLocked(chunk)
		s.emitFrameLocked()
	} else {
		// Raw path — §11.1 ordering preserved verbatim.
		s.fanOutLocked(chunk)
		s.writeModelLocked(chunk)
	}
	s.dirty = true
	if now := time.Now(); now.Sub(s.lastFgSampleAt) >= foregroundSampleInterval {
		s.lastFgSampleAt = now
		s.sampleForegroundLocked()
	}
}

// useModelDrivenLocked: the flag, gated by model health — a degraded model
// (any recovered parse panic) can no longer be the source of truth, so the
// session flips to raw streaming for its remaining lifetime. Caller holds s.mu.
func (s *Session) useModelDrivenLocked() bool {
	return s.modelDriven && s.model != nil && s.modelPanics == 0 && s.emitter != nil
}

// emitFrameLocked derives one frame from the model and fans it out: a diff
// frame normally, a snapshot keyframe when the emitter demands one (unprimed,
// resize, alt flip, scrollback shrink). Caller holds s.mu. Task 7 replaces the
// per-chunk call with the adaptive frame clock.
func (s *Session) emitFrameLocked() {
	data, needKeyframe := s.emitLocked()
	if needKeyframe {
		redraw := s.serializeLocked()
		if len(redraw) == 0 {
			return // serialize panicked → modelPanics bumped → raw fallback next chunk
		}
		s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true})
		s.primeLocked()
		return
	}
	if len(data) > 0 {
		s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: data})
	}
}

// emitLocked / primeLocked wrap the emitter in the same §8.5 recover backstop
// as every other model access. A panic bumps modelPanics, flipping the session
// to raw fallback.
func (s *Session) emitLocked() (data []byte, needKeyframe bool) {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
			data, needKeyframe = nil, false
		}
	}()
	return s.emitter.Emit(s.model)
}

func (s *Session) primeLocked() {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
		}
	}()
	s.emitter.Prime(s.model)
}
```

4. Refactor `fanOutLocked` so both paths share the overflow logic:

```go
func (s *Session) fanOutLocked(chunk []byte) {
	s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: chunk})
}

func (s *Session) fanOutFrameLocked(frame OutputFrame) {
	var overflow []*client
	for cl := range s.clients {
		select {
		case cl.send <- frame:
		default:
			overflow = append(overflow, cl)
		}
	}
	for _, cl := range overflow {
		delete(s.clients, cl)
		close(cl.send)
	}
}
```

(ADAPT to the real overflow-branch body — copy it verbatim, including the close/cleanup details around line 628.)

5. `Resize`: after the model reshape, add `if s.emitter != nil { s.emitter.Invalidate() }` — the next emit produces a keyframe. (The PR #26 client resync request also lands here as a `Resync()` call, which under the flag should ALSO emit a keyframe: change `Resync` to call `s.emitFrameLocked()` when `useModelDrivenLocked()` after invalidating — one mechanism.)

6. Raw-mode degraded logging: in `useModelDrivenLocked` the flip is silent per call; log ONCE by adding a `modelDrivenFellBack bool` field set+logged (via `fmt.Fprintf(os.Stderr, ...)` matching existing session logging style) on first flip.

- [ ] **Step 5: Run the new and existing session suites**

Run: `cd api && go test -tags noEmbed -race ./internal/engine/terminal/... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"`
Expected: all `ok`. The raw-path tests must be untouched-green (flag defaults only apply via engine `Create`; direct `session.New` spawns stay raw).

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/terminal/modeldriven_dev.go api/internal/engine/terminal/modeldriven_release.go api/internal/engine/terminal/modeldriven_flag_test.go api/internal/engine/terminal/terminal.go api/internal/engine/terminal/internal/session/session.go api/internal/engine/terminal/internal/session/session_testseams.go api/internal/engine/terminal/internal/session/session_modeldriven_test.go
git commit -m "feat(terminal): model-driven session output behind flag with raw fallback (P2)"
```

---

### Task 7: Adaptive frame clock

**Files:**
- Modify: `api/internal/engine/terminal/internal/session/session.go`
- Test: `api/internal/engine/terminal/internal/session/session_modeldriven_test.go` (append)

**Interfaces:**
- Consumes: `emitFrameLocked` (Task 6).
- Produces: per-chunk emission replaced by: immediate emit when ≥8ms since last emit, else one trailing timer emit at the 8ms boundary. `s.teardownLocked` (or the session's existing teardown path — find it) stops the timer.

- [ ] **Step 1: Write the failing tests**

Append to `session_modeldriven_test.go`:

```go
func TestModelDriven_BurstCoalescesFrames(t *testing.T) {
	s, err := NewModelDriven("sid-md-burst", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 2000)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	// settle: drain attach snapshot + prompt
	_, _ = collect(ch, 500*time.Millisecond)

	// A burst of 200 lines in one command: with the 8ms clock the client must
	// receive far fewer frames than lines.
	require.NoError(t, s.Write([]byte("seq 1 200\n")))
	frames := 0
	deadline := time.After(3 * time.Second)
	gotLast := false
	for !gotLast {
		select {
		case f := <-ch:
			frames++
			if strings.Contains(string(f.Data), "200") {
				gotLast = true
			}
		case <-deadline:
			t.Fatal("burst output never completed")
		}
	}
	assert.Less(t, frames, 100, "8ms clock must coalesce a 200-line burst (got %d frames)", frames)
}

func TestModelDriven_TrailingTimerFlushesFinalState(t *testing.T) {
	s, err := NewModelDriven("sid-md-trail", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, _ = collect(ch, 500*time.Millisecond)

	require.NoError(t, s.Write([]byte("echo TRAILING-EDGE-OK\n")))
	data, _ := collect(ch, 2*time.Second)
	assert.Contains(t, data, "TRAILING-EDGE-OK",
		"output arriving inside the coalesce window must still flush via the trailing timer")
}
```

- [ ] **Step 2: Run to verify current behavior** (both may already pass with per-chunk emission from Task 6 — the burst test is the one that must KEEP passing after the clock lands; run and note results):

Run: `cd api && go test -tags noEmbed -race -run TestModelDriven ./internal/engine/terminal/internal/session/ -v`

- [ ] **Step 3: Implement the clock**

In `session.go`, add fields:

```go
	// Adaptive frame clock (spec §3.3): emits immediately when the last emit
	// is older than minEmitInterval (interactive echo stays un-batched), else
	// arms one trailing timer at the boundary so bursts coalesce. Guarded by
	// s.mu; the timer callback re-locks.
	lastEmitAt time.Time
	emitTimer  *time.Timer
```

Add near `foregroundSampleInterval`:

```go
// minEmitInterval is the model-driven frame clock (spec §3.3): interactive
// deltas emit immediately; bursts coalesce to at most one frame per interval.
const minEmitInterval = 8 * time.Millisecond
```

Replace the Task 6 per-chunk `s.emitFrameLocked()` call in `pumpStep` with `s.scheduleEmitLocked()`:

```go
// scheduleEmitLocked implements the adaptive clock. Caller holds s.mu.
func (s *Session) scheduleEmitLocked() {
	if time.Since(s.lastEmitAt) >= minEmitInterval {
		s.lastEmitAt = time.Now()
		s.emitFrameLocked()
		return
	}
	if s.emitTimer != nil {
		return // trailing emit already armed
	}
	delay := minEmitInterval - time.Since(s.lastEmitAt)
	s.emitTimer = time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.emitTimer = nil
		select {
		case <-s.done:
			return // session tore down while the timer was in flight
		default:
		}
		if !s.useModelDrivenLocked() {
			return
		}
		s.lastEmitAt = time.Now()
		s.emitFrameLocked()
	})
}
```

Teardown: find the session's teardown/shutdown path (where `s.done` is closed / clients are drained) and add:

```go
	if s.emitTimer != nil {
		s.emitTimer.Stop()
		s.emitTimer = nil
	}
```

under the same lock hold that closes `s.done`.

- [ ] **Step 4: Run session suite with race detector (timer + lock interplay is exactly what -race is for)**

Run: `cd api && go test -tags noEmbed -race ./internal/engine/terminal/internal/session/ 2>&1 | tail -2`
Expected: `ok`, no race reports.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/terminal/internal/session/session.go api/internal/engine/terminal/internal/session/session_modeldriven_test.go
git commit -m "feat(terminal): adaptive 8ms frame clock for model-driven emission (P2)"
```

---

### Task 8: Device-query answers from the model

**Files:**
- Modify: `api/internal/engine/terminal/internal/model/vt_emu.go` (response tap), `api/internal/engine/terminal/internal/model/model.go` + `vt_model.go` (interface passthrough), `api/internal/engine/terminal/internal/session/session.go` (PTY write-back)
- Test: `api/internal/engine/terminal/internal/session/session_modeldriven_test.go` (append), `api/internal/engine/terminal/internal/model/vt_model_test.go` (append)

**Interfaces:**
- Consumes: `vtEmu.drainResponses` (currently discards the emulator's answers to ESC[6n/DA/OSC-color queries), `s.ptmx`.
- Produces: `TerminalModel` gains `SetResponseSink(func([]byte))`; under the flag the session sinks responses to the PTY — apps that query cursor position keep working even though the client xterm (today's answerer) never sees the query. With the flag off the sink stays nil and behavior is unchanged (client answers, model's copies discarded).

**Why this task exists (spec amendment):** in raw mode the CLIENT xterm answers device queries by writing replies to the PTY. Model-driven clients never see the queries, so the daemon must answer them — and the pinned x/vt already computes the answers into its response pipe; today's drain goroutine throws them away.

- [ ] **Step 1: Amend the spec**

Append to `docs/superpowers/specs/2026-07-03-model-driven-terminal-rendering-design.md`:

```markdown
## 3.8 Device queries (amendment, plan phase)

In raw mode the client xterm answers device queries (CPR `ESC[6n`, DA,
OSC 10/11/12 color queries) by writing replies into the PTY. Model-driven
clients never receive the queries, so the daemon answers them from the model:
x/vt already synthesizes the replies into its response pipe (drained and
discarded today). Under the flag the session installs a response sink that
writes those bytes to the PTY master. Flag off → sink nil → discarded as
today (the client remains the answerer). One answerer at a time, always.
```

- [ ] **Step 2: Write the failing tests**

Model-level, append to `vt_model_test.go`:

```go
func TestResponseSink_ReceivesCPRAnswer(t *testing.T) {
	m, _ := New(20, 5, 100) // ADAPT factory
	t.Cleanup(func() { m.Close() })
	got := make(chan []byte, 4)
	m.SetResponseSink(func(p []byte) {
		cp := append([]byte(nil), p...)
		got <- cp
	})
	m.Write([]byte("\x1b[6n")) // cursor position report query
	select {
	case reply := <-got:
		assert.Regexp(t, `\x1b\[\d+;\d+R`, string(reply), "CPR reply expected")
	case <-time.After(2 * time.Second):
		t.Fatal("no device-query reply reached the sink")
	}
}
```

Session-level, append to `session_modeldriven_test.go`:

```go
func TestModelDriven_CPRQueryAnsweredToPTY(t *testing.T) {
	s, err := NewModelDriven("sid-md-cpr", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, _ = collect(ch, 500*time.Millisecond)

	// `read -s -t 3 REPLY` after emitting ESC[6n: the shell captures whatever
	// comes back on stdin. If the daemon answers, REPLY holds the CPR and the
	// marker prints; with no answer, read times out and prints NOANSWER.
	script := `printf '\x1b[6n'; IFS= read -rs -t 3 -d R REPLY && echo "CPR-ANSWERED" || echo "NOANSWER"` + "\n"
	require.NoError(t, s.Write([]byte(script)))
	data, _ := collect(ch, 5*time.Second)
	assert.Contains(t, data, "CPR-ANSWERED")
	assert.NotContains(t, data, "NOANSWER")
}
```

- [ ] **Step 3: Implement**

`model.go` — add to the `TerminalModel` interface:

```go
	// SetResponseSink installs the receiver for the emulator's device-query
	// answers (CPR, DA, OSC color queries). nil (the default) discards them —
	// correct while a live client xterm is the answerer (raw mode). The sink
	// is called from the model's internal drain goroutine; implementations of
	// the sink must be safe for that (the session's sink only calls
	// ptmx.Write, which is safe concurrently with reads).
	SetResponseSink(sink func(p []byte))
```

`vt_emu.go` — thread a sink through `drainResponses` (it currently reads and discards from the InputPipe): store `sink atomic.Pointer[func(p []byte)]` on `vtEmu`; `drainResponses` loop, after each successful read: load the pointer; if non-nil call it with the bytes, else discard. Add `func (v *vtEmu) SetResponseSink(f func(p []byte))`. `vt_model.go`: forward `func (m *vtModel) SetResponseSink(f func(p []byte)) { m.emu.(*vtEmu).SetResponseSink(f) }` — note the fake/test emulators used by panic tests also satisfy `emulator`; add `SetResponseSink` to the `emulator` interface with no-op implementations on the test fakes (compile errors will point at each one).

`session.go` — in `spawn`, after the model is constructed, when `p.ModelDriven`:

```go
	if p.ModelDriven && s.model != nil {
		ptmx := s.ptmx
		s.model.SetResponseSink(func(reply []byte) {
			// Answer device queries from the model (spec §3.8). ptmx.Write is
			// safe from the drain goroutine; a write error just means the PTY
			// is going away — the reply is moot.
			_, _ = ptmx.Write(reply)
		})
	}
```

- [ ] **Step 4: Run the suites**

Run: `cd api && go test -tags noEmbed -race ./internal/engine/terminal/... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"`
Expected: all `ok` (pay attention to the panic-test fakes compiling).

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-07-03-model-driven-terminal-rendering-design.md api/internal/engine/terminal/internal/model/model.go api/internal/engine/terminal/internal/model/vt_emu.go api/internal/engine/terminal/internal/model/vt_model.go api/internal/engine/terminal/internal/model/vt_model_test.go api/internal/engine/terminal/internal/session/session.go api/internal/engine/terminal/internal/session/session_modeldriven_test.go
git commit -m "feat(terminal): answer device queries from the model under the flag (P2, spec §3.8)"
```

---

### Task 9: Dev divergence canary

**Files:**
- Modify: `api/internal/engine/terminal/internal/session/session.go`
- Test: `api/internal/engine/terminal/internal/session/session_modeldriven_test.go` (append)

**Interfaces:**
- Consumes: `model.New` (a second "client-sim" model), emitted frames.
- Produces: with `CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY=1`, the session mirrors every emitted frame into a shadow client-sim model and logs (stderr, once per divergence burst) when its grid hash differs from the authoritative model's. Zero cost when the env var is unset.

- [ ] **Step 1: Write the failing test**

```go
func TestModelDriven_CanaryStaysSilentOnHealthySession(t *testing.T) {
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY", "1")
	s, err := NewModelDriven("sid-md-canary", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("seq 1 50; echo CANARY-DONE\n")))
	data, _ := collect(ch, 3*time.Second)
	require.Contains(t, data, "CANARY-DONE")
	assert.Equal(t, int64(0), s.CanaryDivergences(), "healthy stream must never diverge")
}
```

Add the counter accessor to the session (exported for tests + engine Stats):

```go
// CanaryDivergences reports how many divergence events the dev canary saw.
func (s *Session) CanaryDivergences() int64
```

- [ ] **Step 2: Implement**

In `session.go`: when spawning with `ModelDriven` and the canary env set, construct a second model (`model.New` with the same dims/scrollback, discard its serializer) as `s.canarySim`. In `emitFrameLocked`, after fan-out: keyframes → `canarySim.Close(); s.canarySim = fresh; canarySim.Write(redraw)`; diff frames → `canarySim.Write(data)`. Then compare a cheap grid hash (FNV over each row's `CellAt(x,y).String()` — extract `gridHash(m TerminalModel) uint64` into `model/diff.go` reusing `snapshotRow`); on mismatch, `s.canaryDivergences++` (atomic) and log once per 100 events: `fmt.Fprintf(os.Stderr, "terminal: model-driven canary divergence session=%s count=%d\n", s.id, n)`. Canary work happens under the existing lock hold — acceptable in dev only; it is skipped entirely (nil `canarySim`) without the env var. `CanaryDivergences` reads the atomic.

- [ ] **Step 3: Run + race**

Run: `cd api && go test -tags noEmbed -race -run TestModelDriven ./internal/engine/terminal/internal/session/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add api/internal/engine/terminal/internal/session/session.go api/internal/engine/terminal/internal/model/diff.go api/internal/engine/terminal/internal/session/session_modeldriven_test.go
git commit -m "feat(terminal): dev divergence canary for model-driven output (P3)"
```

---

### Task 10: Full-suite gate + live verification protocol

**Files:**
- No new files; runs and manual verification.

- [ ] **Step 1: Full engine + full unit suite**

Run: `cd api && go test -tags noEmbed -race ./internal/engine/terminal/... && go test -tags noEmbed ./... 2>&1 | grep -vE "^ok|no test files" | head -5`
Expected: engine green; the only acceptable non-engine failure is the PRE-EXISTING `workspaces` build break owned by the other session (verify it is the same error, not a new one).

- [ ] **Step 2: Benchmarks against budgets**

Run: `cd api && go test -tags noEmbed -bench BenchmarkDiff -benchmem -run '^$' ./internal/engine/terminal/internal/model/`
Expected: Echo < 100µs/op, FullRepaint < 8ms/op, ScrollBurst < 8ms/op. Record the numbers in the PR body.

- [ ] **Step 3: Live verification (dev app, manual/driven)**

Rebuild + hot-restart the dev daemon (memory: build `-tags noEmbed` → copy to `desktop/src-tauri/target/debug/crowbar-api` + `binaries/crowbar-api-aarch64-apple-darwin`, kill old, restart with `CROWBAR_HOME=<worktree>/.crowbar` and `--host unix://`). Then, in the dev app (sash drags and terminal input are drivable via synthetic PointerEvents / paste events — see the 2026-07-03 session techniques):

- [ ] Fresh terminal: prompt renders, echo works, `seq 1 200` scrolls smoothly, scrollback intact.
- [ ] Claude Code (`claude`): boots, renders, types; sash shrink+grow cycles leave the buffer stable (no duplicated screens); viewport pinned.
- [ ] `vim` (or `less`): opens (device queries answered — this exercises Task 8), navigates, quits cleanly, screen restores.
- [ ] `location.reload()`: terminal content restores, input round-trips.
- [ ] Workspace switch away/back: re-attach snapshot correct.
- [ ] `CROWBAR_TERMINAL_MODEL_DRIVEN=0` restart: everything above still works on the raw path.
- [ ] Canary run: `CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY=1` restart, use Claude Code ~10 minutes, `grep canary /tmp/crowbar-daemon.log` → no divergences.

- [ ] **Step 4: Ship**

Commit any live-verification fixes (each with its own failing-test-first cycle), then cherry-pick the task commits onto a clean branch off `origin/develop` (`feat/terminal-model-driven-rendering`) via a scratch worktree — the workspace branch history is post-squash-stale and carries another session's work; NEVER push the workspace branch. Open the PR to `develop` with the benchmark numbers and the live-verification checklist in the body.

---

## Self-review notes (completed)

- Spec §3.2 "clear-screen invalidates" was refined in Task 1/2: ED2 is an ordinary all-lines-dirty diff (cheaper, no client reset); only dims/alt/scrollback-shrink demand keyframes. Spec intent (correctness) preserved.
- Spec gap found and fixed: device-query answering (Task 8 + spec §3.8 amendment).
- Accepted residual to note in the PR: BEL (`\a`) and other pure-passthrough non-state bytes are not modeled, so under the flag the client bell is silent. If it matters, forward BEL detection later; out of scope here.
- Type consistency: `Emit/Prime/Invalidate`, `useModelDrivenLocked`, `emitFrameLocked`, `scheduleEmitLocked`, `fanOutFrameLocked`, `SetResponseSink`, `CanaryDivergences` — names match across tasks.
- ADAPT markers are deliberate instructions to copy exact existing signatures (model factory, panic seam, teardown site), not placeholders for unknown design.
