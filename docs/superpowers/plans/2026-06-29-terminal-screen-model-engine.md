# Terminal Engine: VT Screen Model + Serialize-on-Attach

- **Status:** Design locked, ready for implementation
- **Author:** Principal engineer (terminal engine)
- **Date:** 2026-06-29
- **Area:** `api/internal/engine/terminal/`
- **Supersedes:** the raw-ring replay path (`replay_sanitize.go`, `dec_modes.go`, the
  `Attach` CAN preamble, the lazy-grow `RingBuffer`)

---

## 1. Problem statement

### 1.1 What we do today

Each PTY session keeps a `RingBuffer` of the most recent raw PTY output bytes
(`internal/session/ring.go`, ceiling `defaultRingSize = 1 MiB`, lazy-grown from
4 KiB). On client attach — including **re-attach** after a workspace switch, and
**restore** after a daemon restart — the daemon **replays those raw bytes** into a
freshly created, empty `xterm.js` instance on the client to reconstruct the screen:

```
// session.go Attach()
snap := sanitizeReplaySnapshot(s.ring.Snapshot())   // regex-strip replay-unsafe bytes
snap = append(snap, 0x18)                             // CAN: abort any dangling sequence
cl.send <- OutputFrame{Data: snap}
if !s.isIdleLocked() {
    cl.send <- OutputFrame{Data: s.decModes.preamble()}  // re-assert DEC private modes
}
```

The fresh `xterm` is a **stateful VT parser**. Feeding it a *suffix of a historical
byte stream* is categorically different from feeding it the *current screen*. The
ring holds only the last N bytes, so the replay both **begins** and **ends** at
arbitrary points in the stream, and re-drives the parser through state it has
already passed through. This produces a recurring family of bugs.

### 1.2 The bug classes (why raw replay is *fundamentally* wrong)

These are not incidental defects; they are inherent to replaying history into a
stateful parser. Each one already has a patch that "undoes one symptom":

1. **Re-answered device queries.** Apps emit Device Attributes (`ESC[c` / `ESC[>c`),
   Device Status Report (`ESC[6n` cursor-position, `ESC[5n` status), and OSC color
   queries (`ESC]11;?BEL`). On first emission the *real terminal* answered them and
   the *app* consumed the answer. On replay, the fresh `xterm` **answers them again**,
   but now no app is reading — so the reply (`^[[?1;2c`, `^[[40;1R`, `]11;rgb:...`)
   is typed into the **restored shell's prompt as visible garbage**.
   *Patch that fights this:* `replay_sanitize.go` regexes #1, #2, #3.

2. **Stale / garbled titles.** Apps set the tab title via `OSC 0/2 ; <text> BEL/ST`.
   Replaying a *historical* title re-applies a title that is no longer current (e.g.
   a previous `git` command's title), and a zsh prompt that embeds a full ANSI-colored
   prompt inside the OSC parameter replays as a garbled tab.
   *Patch that fights this:* `replay_sanitize.go` regex #4 (with its baroque
   "ESC-that-is-not-ST inside the body" carve-out).

3. **Lost or stale private modes.** The one-time `DECSET` that enables mouse tracking
   / application-cursor-keys / alt-screen is emitted *once* at app startup and is
   usually **evicted from the bounded ring** long before re-attach — so the fresh
   `xterm` never learns the app is in application-cursor-keys mode and arrows break.
   Conversely, replaying a *stale* `DECSET` onto a restored **shell** (no app to
   consume the events) leaks mouse/focus escapes into the prompt.
   *Patch that fights this:* `dec_modes.go` (`decModeTracker.observe`) plus the
   `Attach` preamble plus `replay_sanitize.go` regex #5 (strip historical mode bytes,
   then re-assert the net-active set).

4. **Dangling / truncated sequences.** The ring can **end mid-sequence** — e.g. an
   `OSC 2 ; title` whose `BEL`/`ST` terminator was evicted. Replayed as-is, the fresh
   `xterm` parser enters the OSC-string state and **never leaves it**, swallowing the
   live stream *and* the DEC-mode preamble into a never-terminated title (the
   "garbled-tab eats everything" bug). The ring can equally **begin mid-sequence**.
   *Patch that fights this:* the `snap = append(snap, 0x18)` CAN byte in `Attach`.

5. **No reflow / wrap correctness.** Raw bytes encode *one* terminal width. If the
   client re-attaches at a different size, replayed absolute cursor moves and
   autowrap land on the wrong cells.

Every patch above is a **point fix for one symptom of one root cause**: the daemon is
replaying *history* through a *stateful parser* instead of describing the *current
state*. The patches interact (the sanitizer strips bytes the mode-tracker then has to
re-synthesize; the CAN byte exists only because the sanitizer can't see across the
ring boundary). The pile is fragile, under-tested at the seams, and **grows by one
regex per newly discovered escape sequence** forever. This approach is rejected.

### 1.3 The fix (one sentence)

Run a **headless VT emulator per session** in the daemon, feed it every PTY byte, and
on attach **serialize the current screen** into one clean, well-formed, ground-state
ANSI redraw — emit **state, not history**. This is exactly how `tmux` reattaches a
client. It kills the entire bug class at the root because a serialized snapshot
contains **no queries to re-answer, no historical titles, no dangling sequences, and
no width ambiguity** — by construction.

---

## 2. Goals / non-goals

### 2.1 Goals

- **G1.** Replace raw-byte replay with a daemon-side VT screen model + serialize-on-attach.
- **G2.** A re-attach payload contains **no DA/DSR query replies, no raw historical OSC
  re-emission, no dangling/unterminated sequence** — verifiable by black-box test.
- **G3.** A round-trip (serialize → feed into a fresh same-size `xterm`) reproduces the
  **identical visible buffer** (grid cells, SGR, cursor pos/shape/visibility, title,
  active private modes, alt-screen state) **and the identical scrollback** — the
  reconstructed xterm's scrollback **line-count equals the model's** and the **newest
  scrolled-off line matches**, so no history (least of all the most-recent lines) is lost
  on attach. Verifiable by black-box test (§13.3).
  - **G3 carve-out (alt-screen branch — explicit, accepted, NOT full fidelity).** When the
    session is in the alternate buffer at attach (`shadow.altScreen` true), G3 covers **only
    the active alt grid + cursor + modes + title**. It does **NOT** cover the saved *primary*
    buffer or its scrollback: per the locked decision (§10(b)) we serialize only the active
    alt grid and `?1049h`, and x/vt does not surface the inactive primary buffer for us to
    re-paint underneath it. **Documented consequence (accepted):** if the alt-screen app
    later exits (`?1049l`) *after* a re-attach, the new xterm restores the **empty** primary
    buffer it started with — the pre-app shell screen and its scrollback that existed before
    the app launched are **lost** for that re-attached client (the app repaints its own alt
    content while running; only the post-exit primary is affected). This is the single,
    bounded fidelity gap in the whole design, it is **scoped to alt-reattach-then-exit only**,
    and it is asserted (as a non-corrupting, clean-empty outcome — never garbage) by
    `TestRegression_ReattachDuringAltScreenThenExit` (§13.3). All non-alt attaches retain
    full G3 fidelity.
- **G4.** The emulator is **quarantined behind an internal interface** (`TerminalModel` /
  `Serializer`) so `charmbracelet/x/vt` is swappable for `vito/midterm` with no caller
  changes.
- **G5.** **Scrollback depth = the user's `terminalScrollback` setting (default 10000
  lines)**, stored compactly so depth does not blow the memory budget.
- **G6.** **100% test coverage** on the new packages; black-box `TestRegression_*` in
  `api/tests` per repo convention; a conformance harness that diffs our serialized output
  against `xterm.js` `SerializeAddon` across real-app fixtures.

### 2.2 Non-goals (explicit)

- **NG1. The live path is untouched.** `pumpStep` still fans out **raw PTY bytes** to
  every attached client with **zero added latency and zero transformation**. Claude Code,
  vim, htop, less, and DOOM behave **byte-for-byte identically** live. *Only the
  attach/re-attach snapshot changes.* We never re-render the live stream. **One honest
  caveat (§8.3):** `Attach` and the 10 s cadence flush each hold `s.mu` for the duration
  of one `Serialize` (a grid `Render()` — pre-serialized scrollback is a memcpy, not a
  render), which briefly excludes `pumpStep` for that session. This is a **bounded sub-
  millisecond stall** (§8.3 bounds it at the grid-render cost, ≈ tens of µs to low-
  hundreds of µs at realistic sizes), not "zero," and it affects **only the one session
  being attached/flushed**, not the steady-state fan-out of any other session. We
  explicitly accept this bounded per-attach/flush stall as the price of the single-lock
  atomicity (§8.3); it is far below perceptible latency and never accumulates.
- **NG2. No scrollback reflow on resize.** We do not re-wrap stored scrollback when the
  terminal is resized (matches `tmux`, iTerm2 default, and most terminals). See §10(a).
- **NG3. No persisted-state migration.** Pre-production, no users. Old `.buf` files
  (raw bytes) are not migrated; dev state is cleared. See §12.
- **NG4. No new client/FE protocol.** The wire frame (`outputMsg{sessionId,data,isInput}`)
  is unchanged. The serialized redraw is just bytes in `data`, exactly like replay was.
- **NG5. We do not parse the live stream on the client.** All modeling is daemon-side.

---

## 3. Architecture overview & data flow

### 3.1 Two paths, sharply separated

```
                         ┌──────────────────────────── LIVE PATH (unchanged, raw) ──────────────┐
                         │                                                                       │
  PTY master ──Read()──► pumpStep(chunk)                                                          │
   (creack/pty)          │                                                                        ▼
                         ├─(1)─► fanOutLocked(chunk)  ──►  client.send chan  ──►  writePump  ──► WS ──► xterm.js (live clients)
                         │                                                                        ▲
                         └─(2)─► model.Write(chunk)   [headless VT emulator: grid+scrollback+    │
                                  updates current      cursor+modes+title]                        │
                                  screen state                                                    │
                                                                                                  │
                         ┌──────────────────── ATTACH PATH (new, serialized) ─────────────────────┘
                         │
   New client Attach() ─► snapshot := Serializer.Serialize(model)   [ONE clean ground-state redraw]
                         └─► client.send <- snapshot                ──►  writePump  ──► WS ──► xterm.js (the new client only)
                              then register client for live fan-out
```

**Critical ordering inside `pumpStep` (the hot loop):** fan-out **first**, model update
**second**, both under `s.mu` (see §8). The model update is pure in-memory VT parsing of
a buffer the daemon already owns; it never does IO and never blocks a `select`-send. Live
sends are enqueued before the model is touched, so the model can **never** add latency to
live output.

### 3.2 What the model is

A `TerminalModel` is a headless terminal emulator: it has a width × height **cell grid**
(the visible screen), a bounded **scrollback** of evicted lines, a **cursor**
(position/shape/visibility/color), the active **DEC private modes**, the **scroll region**,
the **alt-screen** flag, and the **title/icon name**. Every PTY byte is parsed into grid
mutations. It is the daemon's authoritative mirror of what a correct terminal *would*
currently show. We never render it for live clients; we only **serialize** it on attach.

### 3.3 What changes per file

| File | Change |
|---|---|
| `internal/session/session.go` | `ring *RingBuffer` → `model TerminalModel`; `pumpStep` fan-out-first then `model.Write` (under `defer s.mu.Unlock()` + a recover boundary around `model.Write`, §8.2); `Attach` replaces sanitize+CAN+preamble with `serializer.Serialize(model)`; `Snapshot()` returns serialized state + size header; `New` gains `initialCols,initialRows,scrollbackLines` from the **transient create DTO** (create path, §4.2/§9.1); `NewRestored` takes the **raw blob** and derives size from the §12 header (§4.2/§12); `NewPlaceholder`/`LoadPlaceholder` hold **only the raw blob — no `vtModel`** (§4.2/§12); `Resize` sizes PTY and model in lockstep; `OnForegroundReset` on the foreground-app-died edge, sampled per §11.1. |
| `internal/session/ring.go` | **deleted** |
| `internal/session/replay_sanitize.go` | **deleted** |
| `internal/session/dec_modes.go` | **deleted** |
| `internal/session/model/` (new pkg) | `TerminalModel` + `Serializer` interfaces, the `x/vt` adapter, shadow state. |
| `terminal.go` | `RingCap()`/ring-byte accounting → model memory accounting; `Stats`/`maxTotalRingBytes` semantics updated (§9). |
| `internal/persistence/` | unchanged API; payload is now serialized state (§12). |

---

## 4. Internal interfaces

These live in a **new internal package** `api/internal/engine/terminal/internal/session/model`.
They are the **only** surface `session.Session` touches. The concrete emulator
(`x/vt`, later `midterm`) is never named outside this package.

```go
// Package model defines the daemon-side terminal screen model and its
// serializer, quarantining the concrete headless VT emulator (charmbracelet/x/vt
// today, vito/midterm tomorrow) behind two interfaces so the session engine never
// depends on a specific emulator. The TerminalModel interface is deliberately
// minimal — it is the SESSION-facing surface: the session feeds bytes in
// (Write/Resize/OnForegroundReset), asks for size/title/teardown, and (for the §8.3
// mid-sequence attach boundary) reads the buffered partial-sequence bytes via
// PendingInput; nothing the session does not need leaks across THIS boundary. The Serializer necessarily reads
// far more of the screen (visible grid cells, cursor, alt-screen flag, full-depth
// scrollback, and the shadow private-mode/charset/cursor/colour state), but it does so
// ONLY against its PAIRED concrete model INSIDE this package, never through the
// session-facing TerminalModel interface — see "Model↔serializer read contract" below.
// That is what reconciles "the session sees a minimal interface" with "the serializer
// reads the whole screen": the two facts live on opposite sides of the package wall.
package model

// TerminalModel is a headless terminal emulator: an authoritative, in-memory
// mirror of the screen a correct terminal would currently display for a session.
// It is fed every PTY output byte and maintains the visible cell grid, bounded
// scrollback, cursor, active DEC private modes, scroll region, alt-screen flag,
// and title. It performs no IO. Implementations are NOT required to be
// goroutine-safe; the owning Session serialises all access under s.mu (see §8).
type TerminalModel interface {
	// Write feeds one chunk of raw PTY output into the emulator, advancing screen
	// state. It must never block, never perform IO, and never answer device
	// queries (a headless emulator has no client to answer). Partial escape
	// sequences split across chunk boundaries must be buffered internally and
	// resumed on the next Write — the session passes PTY reads through verbatim.
	Write(p []byte)

	// Resize changes the grid dimensions to cols×rows. Per §10(a) it does NOT
	// reflow stored scrollback; it only reshapes the live grid. Called when the
	// client resizes (after the PTY SIGWINCH) so the model and PTY stay in lockstep.
	// The session calls Resize and pty.Setsize under one s.mu hold with no
	// intervening Write, preserving the model==PTY size invariant (§4.2).
	Resize(cols, rows int)

	// OnForegroundReset clears transient, app-owned screen state (alt-screen, mouse,
	// focus, bracketed-paste, app-cursor-keys, origin, cursor shape; autowrap back to
	// ON) WITHOUT touching grid/scrollback/title/cursor. The session calls it on the
	// edge where the PTY foreground process group returns to the shell (a foreground
	// app exited, possibly via SIGKILL without emitting its own resets), so a later
	// Serialize cannot leak stale modes into an idle prompt. Idempotent. See §11.1.
	OnForegroundReset()

	// PendingInput returns the raw bytes of any INCOMPLETE escape/control sequence the
	// parser is currently mid-way through buffering (an empty slice when the parser is in
	// ground state). It is the §8.3 "mid-sequence attach boundary" re-sync surface: those
	// exact bytes were already fanned out RAW to every live client in a prior pumpStep
	// chunk (a kernel PTY read split a write mid-sequence), but Serialize reflects only
	// COMMITTED ground-state screen state and so does NOT contain them — so a freshly
	// attaching client, after the clean redraw, would receive only the SEQUENCE TAIL on its
	// first live chunk and render it as orphaned garbage. Attach appends PendingInput()
	// after the redraw (Appendix A) so the new client's fresh parser enters the IDENTICAL
	// mid-sequence state as the live clients and the tail completes correctly. It is a pure
	// read; the caller holds s.mu. NEVER consumed by Snapshot()/persistence (the .buf must
	// stay self-contained — re-emitting a partial whose tail will never arrive on a fresh
	// restored PTY would re-create the §1.2#4 dangling-sequence bug). Bounded: the adapter
	// retains at most maxPendingPartial (= 4 KiB) of in-flight bytes; a longer un-terminated
	// sequence returns "" (the accepted §8.3 adversarial-flood residual, self-healing on
	// repaint). See §4.1 (source) and §8.3 (boundary contract).
	PendingInput() []byte

	// Title returns the current window title (last OSC 0/2 text), or "" if unset.
	Title() string

	// Cols and Rows return the current grid dimensions.
	Cols() int
	Rows() int

	// HeaderState returns, in one call, ALL FOUR fields the persistence header
	// (`CRWB1 <cols> <rows> <alt> <scrollbackLines>`, §12) needs — and is the SINGLE,
	// authoritative source for that header so `Session.header()` (§12) is buildable with
	// zero further decisions and never re-parses the stream. Two of the four fields are
	// otherwise unreachable from the session side: `alt` (the alt-screen flag, held in the
	// adapter's shadow / emulator and deliberately NOT exposed elsewhere on this
	// session-facing interface) and `scrollbackLines` (the user's configured depth, owned by
	// the adapter's compact-line ring, not stored on the Session). Exposing them through this
	// one narrow accessor — rather than widening the interface with an `IsAltScreen()` getter
	// AND threading `scrollbackLines` onto the Session — keeps the session boundary minimal
	// while making the mandatory header constructible. The x/vt adapter sources cols/rows from
	// `emu.Width()`/`emu.Height()`, `alt` from `emu.IsAltScreen()` (the buffer Render actually
	// paints — see the §4.2 single-source-of-truth note), and `scrollbackLines` from
	// `scrollback.Cap()` (§9.2.2). It performs no mutation; the session calls it under s.mu.
	HeaderState() (cols, rows int, alt bool, scrollbackLines int)

	// Close releases any internal resources. Safe to call once; idempotent.
	Close()
}

// Serializer turns a TerminalModel's current state into ONE clean, well-formed,
// ground-state ANSI byte sequence that, when written to a fresh same-size xterm,
// reproduces the model's visible buffer exactly (cells, SGR, cursor, modes,
// title). The output contains NO device-query requests, NO device-query replies,
// NO historical OSC re-emission beyond the single current title, and NO
// dangling/unterminated sequence. It is the sole on-attach payload.
//
// Serialize is a pure read of the model; it never mutates it. The caller holds
// s.mu across the call (the x/vt model is not thread-safe — see §8).
//
// A Serializer is PAIRED with exactly one concrete TerminalModel implementation (the
// x/vt pair today, the midterm pair tomorrow). The matched pair is built together by
// model.New (§5.3/§5.4), so the concrete type a Serializer receives is guaranteed.
// Serialize recovers its paired concrete model via a type assertion (m.(*vtModel) for
// the x/vt backend) and reads the render surface enumerated in "Model↔serializer read
// contract" below. This read coupling is entirely INTERNAL to package model — the
// session never sees it — so the "nothing else leaks" guarantee on the session-facing
// interface is unaffected.
type Serializer interface {
	// Serialize returns the redraw bytes for m. The exact byte order is specified
	// in §6. Implementations MUST be deterministic for a given model state so the
	// conformance harness can diff byte-for-byte. Serialize type-asserts m to its
	// paired concrete model; because model.New constructs model+serializer together,
	// the assertion cannot fail in production (a mismatch is an unreachable
	// programming error and panics — caught by the session's `serializeLocked` recover
	// wrapper (§8.2/§8.5), which every Serialize call site routes through, NOT by a bare
	// `defer Unlock` (an unlock does not recover)).
	Serialize(m TerminalModel) []byte
}

// New constructs the matched (TerminalModel, Serializer) pair for the active backend.
// It is the SINGLE place the concrete backend is named (§5.3); flipping it to midterm
// (§5.4) is a one-line change. Returning BOTH halves from one constructor is what makes
// the Serializer's downcast of its paired model sound — the session stores the two
// returned interface values (s.model, s.serializer) and can never combine a model of
// one backend with a serializer of another.
func New(cols, rows, scrollbackLines int) (TerminalModel, Serializer)

// ModelHealth is an OPTIONAL interface a TerminalModel MAY also implement to expose its
// parse-health to Stats WITHOUT widening the minimal session-facing TerminalModel surface
// and WITHOUT the session downcasting to the concrete *vtModel (Issue). The session reads
// it with a guarded type-assert — `if h, ok := s.model.(ModelHealth); ok { … }` — so a
// backend that does not implement it simply contributes no degraded/parsePanics detail.
// The x/vt adapter (*vtModel) implements it (Degraded/ParsePanics, §4.1). This is how
// "adapter degraded surfaced in Stats" (§3.3/§4.1) is reconciled with "the session holds
// only the minimal TerminalModel": health is a separate, optional capability, not a core
// method every implementation must provide.
type ModelHealth interface {
	Degraded() bool    // true once the model recovered ≥1 parse panic (screen blanked-and-reparsed)
	ParsePanics() int  // count of recovered emu.Write panics this session
}
```

#### Model↔serializer read contract (the explicit read surface)

The `TerminalModel` interface above is the **session boundary**. The serializer needs a
**much wider read surface** than that interface exposes, and §6/§13.1 are only
implementable once that surface is named concretely. We resolve it with **per-backend
pairing + an in-package downcast**, not by widening the session-facing interface:

- `model.New` returns a matched pair built from the same backend. For x/vt that is
  `(*vtModel, *vtSerializer)`; for midterm `(*midtermModel, *midtermSerializer)` in the
  sibling file (§5.4).
- `vtSerializer.Serialize(m TerminalModel)` begins with `vm := m.(*vtModel)` and reads
  the render surface **directly off the concrete `*vtModel`**. The exact, enumerated set
  it consumes (and nothing more) is:

  | §6 step(s) | Concrete read off `*vtModel` |
  |---|---|
  | 7 (α body) | `vm.emu.Render()` — grid-only ANSI for the active buffer |
  | 7 (β body), 13 repaint | `vm.emu.CellAt(x, y)` — per-cell `uv.Cell` for the visible grid |
  | 3, 5-gate, 7-buffer-select | `vm.emu.IsAltScreen()` — the **single source of truth** for the alt flag (`alt`), computed ONCE at entry; `vm.shadow.altScreen` is a cross-check only (see "Single source of truth for buffer selection" below) |
  | 13 | `vm.emu.CursorPosition()` — real cursor cell |
  | 5 | `vm.scrollback` (the `lineRing`) — `(bytes, wrapped)` pairs, full depth, already compact |
  | 2 | `vm.shadow.{fg,bg,cursorColor}` + `{fgSet,bgSet,cursorColorSet}` |
  | 8, 9, 12 | `vm.shadow.modes` (the tracked DEC private-mode map) |
  | 10 | `vm.shadow.{g0,g1,glLock}` |
  | 11 | `vm.shadow.{scrollRegionSet,scrollTop,scrollBottom}` |
  | 14, 15 | `vm.shadow.{cursorShape,cursorBlink,cursorShapeSet,cursorVisible}` |
  | 16 | `vm.shadow.{title,iconName}` (through `sanitizeOSCText`) |
  | sizing | `vm.emu.Width()` / `vm.emu.Height()` |

  These are the **only** members the serializer touches; `Serialize` performs no
  mutation. The `vtModel` fields it reads (`emu`, `shadow`, `scrollback`) are
  package-private, so this contract cannot be exercised from outside `model/`.
- **Single source of truth for buffer selection (the alt flag — Issue: callback-miss
  divergence).** `Serialize` computes the alt flag **exactly once at entry** as
  `alt := vm.emu.IsAltScreen()` and uses *that one value* for **all three** alt-dependent
  decisions: the step-3 `?1049h` gate, the step-5 scrollback-skip gate, and the step-7
  buffer selection. `vm.emu.IsAltScreen()` is chosen as authoritative — not
  `vm.shadow.altScreen` — because the step-7 body (`vm.emu.Render()` / `vm.emu.CellAt`) reads
  the emulator's *physically active* buffer, so deriving the step-3 and step-5 gates from any
  other value could emit `?1049h` + skip scrollback while `Render` paints the primary grid (or
  vice-versa) — exactly the contradictory payload §11.1 warns about. Because all three gates
  read the same `alt`, step 5 and step 7 **cannot** disagree, by construction. The §6 step
  conditions written as "`shadow.altScreen`" / "`!shadow.altScreen`" denote this reconciled
  `alt` value, not a second independent read.
  - **Mismatch handling (`alt != vm.shadow.altScreen`).** The shadow flag is still read, but
    **only as a consistency cross-check**. Outside the §11.1 teardown edge the two must agree;
    if they diverge — a callback x/vt failed to deliver at the pinned commit — `Serialize`
    **reconciles to `vm.emu.IsAltScreen()`** (it never trusts the shadow over the painted
    buffer), **writes `vm.shadow.altScreen = alt`** so the shadow re-converges, sets the
    model's sticky `degraded` flag, and logs once at warn (`alt-flag divergence: emu=%v
    shadow=%v`). It does **not** abort or emit a contradictory payload: the redraw is always
    internally consistent with the buffer `Render` paints. A `model/` unit test forces a shadow
    desync (drive emu into alt while suppressing the AltScreen callback) and asserts the payload
    is alt-consistent (`?1049h` present, step 5 skipped, grid = alt) and `Degraded()` is set.
- **No session-facing leak.** The session calls only `Write/Resize/OnForegroundReset/
  Title/Cols/Rows/Close` on `TerminalModel`, and passes the *same* `TerminalModel` value
  to `Serialize`. The downcast and all the reads above happen **inside** `vtSerializer`,
  in package `model`. The session is never coupled to `*vtModel`, so the swap to midterm
  (§5.4) still changes nothing outside `model/`.
- **The `midterm` pair** implements the identical contract: `midtermSerializer.Serialize`
  downcasts to `*midtermModel` and reads midterm's own grid/cursor/mode accessors. Each
  backend's golden-byte file (§13.1) is therefore inherently per-backend; only the §13.2
  buffer-equivalence diff is cross-backend (§5.4).

### 4.1 The `x/vt` adapter shape

`x/vt` exposes screen state two ways:

- **On-demand public getters** (read whenever): `CellAt(x,y)`, `CursorPosition()`,
  `IsAltScreen()`, `Scrollback()` / `ScrollbackLen()` / `ScrollbackCellAt()`,
  `Foreground()` / `Background()` / `CursorColor()`. **NOTE:** the adapter builds x/vt with
  `WithScrollback(0)` (§9.2 blocker resolution), so x/vt's own scrollback getters
  (`Scrollback`/`ScrollbackLen`/`ScrollbackCellAt`) always return empty and are **NOT** used —
  the authoritative scrollback is the adapter's compact `lineRing`, fed by the eviction hook.
- **Push-only via a `Callbacks` struct** (NOT readable on demand): private mode
  enable/disable, `Title`, `IconName`, alt-screen enter/leave, cursor visibility,
  cursor style (shape), cursor color, working directory, scroll-region changes, **and the
  pre-scroll/line-eviction hook** (delivers each evicted line's cells + wrap bit — the SOLE
  scrollback-capture path, a HARD P0 adoption gate, §9.2/§15).

Because the second group is delivered as **events**, the adapter keeps **shadow state**
(§7) updated from callbacks, and `Serialize` reads grid/cursor from the getters and
modes/title/shape from the shadow.

```go
package model

import (
	"image/color" // for app-set default fg/bg/cursor colors (step 2, §7 shadow)

	uv "github.com/charmbracelet/ultraviolet" // uv.Cell in the file-local emulator seam
	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/ansi"
)

// vtModel adapts charmbracelet/x/vt to TerminalModel. It owns a *vt.Emulator and a
// shadow struct fed by vt.Callbacks for state x/vt only pushes (modes, title,
// shape, alt-screen, scroll region). The session serialises all access under s.mu,
// so we use the raw *vt.Emulator (NOT vt.SafeEmulator); the lock IS our safety.
// (If a future code path needs concurrent access we switch the field type to
// vt.SafeEmulator — a one-line change confined to this file. See §8.)
// vtScrollbackWindow is the number of lines x/vt is allowed to retain in its OWN
// (cell-based) scrollback. It is FIXED at 0: the pre-scroll/eviction HOOK is the SOLE
// scrollback-capture mechanism (§9.2, a HARD P0 adoption gate), so x/vt is ALWAYS built
// with WithScrollback(0). The hook hands the adapter each line WITH its wrap flag at the
// instant it scrolls off the primary grid — before x/vt would drop it — so NO retained
// cell window is needed and x/vt's cell cost is independent of scrollbackLines (the
// user's depth). The const is kept (= 0) only so §9.4 ModelBytes()'s "retained-window"
// term reads literally as vtScrollbackWindow*cols*bytesPerCell == 0.
//
// The previously-considered ScrollbackLen()-diff fallback (WithScrollback(256)) is
// DELETED as unsound (blocker resolution): diffing ScrollbackLen() AFTER emu.Write and
// reading evicted lines via ScrollbackCellAt can capture at most ~256 (one window) of the
// lines evicted by that Write. Any single Write that scrolls MORE than the window — a
// RESTORE redraw replaying the full saved depth in one model.Write, a multi-screen 64 KB
// flood chunk, or a tiny `CSI Ps S` that scrolls thousands of lines at once — would lose
// every eviction older than the last window before the diff could read it, silently
// punching holes in the authoritative compact ring. There is NO cheap per-Write batching
// bound that fixes this (a 1-byte LF or a few-byte `CSI 99999 S` can each evict
// unboundedly many lines), so the hook is the ONLY correct mechanism. If the pinned x/vt
// commit exposes no such hook and no thin wrapper around its scroll path can synthesise
// one, x/vt is NOT adopted → midterm (§5.4 / §15 exit-gate item 7).
const vtScrollbackWindow = 0

// emulator is the NARROW, FILE-LOCAL seam over the concrete headless VT emulator
// (*vt.Emulator today). vtModel holds this interface — not *vt.Emulator — for two
// reasons: (a) it makes the §8.2 parse-panic RECOVER branch (and recreateEmu) reachable
// from a unit test by substituting an emulator whose Write panics on a sentinel byte
// (feeding real bytes that deterministically panic an untagged emulator is NOT
// guaranteed, so without the seam the recover branch is unreachable and 100% coverage
// (§13.4) is unattainable); and (b) it keeps the concrete emulator type quarantined.
// It is UNEXPORTED and never leaves package model — the session-facing TerminalModel
// boundary is unchanged. The method set is exactly what vtModel + vtSerializer call
// (the §4.2 read-contract surface). Production ALWAYS uses the *vt.Emulator-backed vtEmu,
// with NO build tag. Crucially this seam adds NO lock and reconciles with §8.1's
// "raw *vt.Emulator, not SafeEmulator" rationale: vtEmu is a zero-state PASS-THROUGH, so
// s.mu remains the sole synchronisation and no double-lock is reintroduced.
type emulator interface {
	Write(p []byte)
	Resize(cols, rows int)
	Width() int
	Height() int
	Render() []byte
	CellAt(x, y int) uv.Cell
	CursorPosition() (x, y int)
	IsAltScreen() bool
	// Per-row soft-wrap signal for the VISIBLE grid — read at PAINT time, on the bounded
	// rows×cols grid (no eviction ceiling). RowWrapped reports whether visible grid row y
	// soft-wraps into y+1 (§9.2 blocker resolution; there is no adapter-maintained side array).
	// EVICTED scrollback rows do NOT use a getter: their cells AND wrap bit are delivered by the
	// pre-scroll/eviction HOOK at the instant they scroll off (the Callbacks entry wired in
	// buildEmu), because x/vt is built WithScrollback(0) — there is no retained-window scrollback
	// to read (and a retained-window getter would carry the per-Write capture ceiling the diff
	// fallback had, §9.2). x/vt exposing BOTH {the eviction hook delivering each evicted line's
	// cells + wrap bit} AND {a usable visible-grid wrap signal — RowWrapped, or a Render() that
	// emits wrapped rows continuously} is a HARD P0 ADOPTION GATE (§15 exit-gate item 7): if the
	// pinned commit exposes neither and no thin wrapper can synthesise them, x/vt is NOT adopted
	// → midterm (§5.4). The pinned-commit getter/hook names are recorded by the spike; any shim
	// lives in vtEmu.
	RowWrapped(y int) bool
}

// vtEmu wraps *vt.Emulator to satisfy the file-local emulator interface. It carries NO
// extra state and NO lock — methods whose x/vt signature matches the seam are PROMOTED by
// the embedded pointer; any getter whose pinned-commit x/vt signature differs (e.g. a
// CursorPosition that returns a Position struct rather than (x, y int)) gets a tiny explicit
// shim method here that adapts the shape — pure translation, still no state and no lock. So
// wrapping is free and does NOT reintroduce the double-lock the raw-emulator choice (§8.1)
// avoids. (The P0 spike pins the exact x/vt getter signatures, §15; any shim is recorded in
// this file only.) If a future path ever needs concurrent access, the one-line flip is to
// embed *vt.SafeEmulator here instead (still file-local, §8.1).
type vtEmu struct{ *vt.Emulator }

// newEmulator constructs the production emulator. It is a PACKAGE-LEVEL VAR (not a plain
// func) ONLY so a unit test can substitute a panicking emulator to drive the §8.2 recover
// branch; production never reassigns it and it carries NO build tag. buildEmu is its sole
// caller, so the *vt.Emulator construction (callbacks + scrollback window) lives in exactly
// one place behind this var.
var newEmulator = func(cols, rows int, cb vt.Callbacks, window int) emulator {
	return &vtEmu{Emulator: vt.NewEmulator(cols, rows, vt.WithCallbacks(cb), vt.WithScrollback(window))}
}

type vtModel struct {
	emu        emulator  // file-local seam over *vt.Emulator (see `emulator` below); NOT the
	                     // concrete type, so the §8.2 parse-panic recover branch is reachable from a
	                     // unit test via an injectable panicking emulator (100% coverage, §13.4)
	shadow     shadowState
	scrollback *lineRing // compact serialized evicted lines — AUTHORITATIVE, full depth
	sbBytes    int64     // running sum of compact-line lengths, for ModelBytes() (§9.4)
	// NOTE (§9.2 "Wrap-flag source", blocker resolution): there is NO per-row wrap side
	// array. The per-evicted-row and per-visible-row soft-wrap flag is read DIRECTLY from
	// x/vt's per-row wrap signal at eviction/paint time (spike outcome (i)). x/vt exposing
	// that signal is a HARD P0 adoption gate (§15 exit-gate item 7): if neither the pinned
	// commit nor a thin pre-scroll-hook wrapper can deliver an evicted row's wrap flag,
	// x/vt is NOT adopted and we fall to the pre-vetted midterm drop-in (§5.4). The adapter
	// never reconstructs wrap state by re-parsing `p` — it does not own the parse.
	degraded   bool      // sticky: set true after a recovered model.Write panic (§8.2 recovery)
	parsePanics int      // metric: count of recovered emu.Write panics this session
	pendingPartial []byte // §8.3 mid-sequence attach re-sync: the raw bytes of the in-flight
	                     // incomplete escape/control sequence at the END of the cumulative stream
	                     // (nil when the parser is in ground state). Sourced in Write by the §4.1
	                     // escape-framing tracker (or x/vt's own partial buffer if the pinned
	                     // commit exposes it), carried across Write calls, capped at
	                     // maxPendingPartial. Read by PendingInput(); Attach appends it after the
	                     // redraw so a freshly-attached client re-syncs to the live boundary.
}

// maxPendingPartial bounds the in-flight partial-sequence bytes the adapter retains for the
// §8.3 mid-sequence attach re-sync. Normal sequences are < 64 B; OSC strings (title, clipboard)
// a few hundred. A still-incomplete sequence longer than this is pathological (an app streaming a
// never-terminated OSC); past the cap PendingInput() returns "" and we accept the bounded,
// self-healing residual (§8.3) rather than re-emit a multi-KB orphaned prefix.
const maxPendingPartial = 4096

// newVTModel builds a vtModel of the given size whose scrollback ceiling is
// scrollbackLines (the terminalScrollback setting). The Callbacks closures write
// only into m.shadow; they run inside emu.Write, which the session already calls
// under s.mu, so no extra synchronisation is needed here.
func newVTModel(cols, rows, scrollbackLines int) *vtModel {
	m := &vtModel{shadow: newShadowState(), scrollback: newLineRing(scrollbackLines)}
	m.emu = m.buildEmu(cols, rows) // single construction path; recreateEmu (§4.1 recovery) reuses it
	return m
}

// buildEmu constructs an emulator of cols×rows with the Callbacks bound to m and the
// fixed drain window, via the newEmulator var (so tests can inject a panicking emulator,
// §8.2). It is the SOLE emulator-construction site (newVTModel AND the panic recovery
// recreateEmu both call it), so the callback wiring exists in exactly one place.
func (m *vtModel) buildEmu(cols, rows int) emulator {
	cb := vt.Callbacks{
		Title:       func(s string) { m.shadow.title = s },
		IconName:    func(s string) { m.shadow.iconName = s },
		AltScreen:   func(on bool) { m.shadow.altScreen = on },
		CursorVisibility: func(v bool) { m.shadow.cursorVisible = v },
		CursorStyle: func(style vt.CursorStyle, blink bool) {
			m.shadow.cursorShape, m.shadow.cursorBlink = style, blink
			m.shadow.cursorShapeSet = true // the app explicitly issued DECSCUSR (gates step 14)
		},
		EnableMode:  func(mode int) { m.shadow.setMode(mode, true) },
		DisableMode: func(mode int) { m.shadow.setMode(mode, false) },
		ScrollRegion: func(top, bottom int) {
			m.shadow.scrollTop, m.shadow.scrollBottom, m.shadow.scrollRegionSet = top, bottom, true
		},
		WorkingDirectory: func(uri string) { m.shadow.workingDir = uri },
		// Charset designates G0/G1 (line-drawing/alt charset). Restored in step 10 so
		// the LIVE stream after re-attach renders correctly (grid cells already hold
		// resolved runes). If x/vt exposes no charset callback the adapter parses the
		// SCS escapes itself in Write (see contract note); default designator is 'B'.
		Charset: func(slot int, designator byte) { m.shadow.setCharset(slot, designator) },
		// LockingShift records which charset is invoked into GL: SI (0x0F) → G0,
		// SO (0x0E) → G1. A DEC line-drawing TUI commonly invokes G1 via SO, so the
		// LIVE stream after re-attach is parsed with GL=G1; step 10 must re-emit the
		// active shift or the post-attach stream mis-renders. If x/vt exposes no
		// locking-shift callback the adapter scans SI/SO bytes in Write itself.
		LockingShift: func(glSlot int) { m.shadow.glLock = glSlot }, // 0 = G0 (SI), 1 = G1 (SO)
		// DefaultColor records an app-issued OSC 10/11/12 so step 2 emits ONLY for a
		// color the app explicitly changed (never overriding the client theme from a
		// bare getter — see §4.1 contract note). slot: 0 = fg, 1 = bg, 2 = cursor.
		DefaultColor: func(slot int, c color.Color) { m.shadow.setDefaultColor(slot, c) },
		// LineEvict is the pre-scroll/eviction HOOK (§9.2, blocker resolution) — the SOLE
		// scrollback-capture path. x/vt fires it synchronously DURING emu.Write / emu.Resize
		// for each physical line that scrolls off the top of the PRIMARY grid, handing the
		// adapter the leaving row's uv.Cells AND its soft-wrap bit BEFORE x/vt drops it. The
		// handler (drainEvictedLine) is guarded `!shadow.altScreen` + full-region and appends
		// encodeRow(cells, wrapped) to the authoritative compact ring. Because the hook fires
		// per line at the moment of eviction, NO single Write can outrun it — there is no
		// retained-window capture ceiling (the deleted diff fallback had one). x/vt is therefore
		// built WithScrollback(0). The pinned-commit field name is recorded by the §15 spike;
		// absence of this hook BLOCKS adoption (→ midterm, §5.4).
		LineEvict: func(cells []uv.Cell, wrapped bool) { m.drainEvictedLine(cells, wrapped) },
	}
	// x/vt retains ZERO cell scrollback (WithScrollback(0)): the authoritative full-depth
	// scrollback is m.scrollback (compact bytes), fed line-by-line by the LineEvict hook above.
	// This is the fix for BOTH the "x/vt keeps its own 24 MB cell scrollback" blowup AND the
	// diff-fallback's per-Write capture ceiling (§9.2). Constructed through the newEmulator var
	// so the production *vt.Emulator (wrapped in vtEmu) is built here while a unit test can swap
	// in a sentinel-panicking emulator (§8.2 recover coverage, §13.4). vtScrollbackWindow is 0.
	return newEmulator(cols, rows, cb, vtScrollbackWindow)
}

// Write feeds a chunk to the emulator. Lines that scroll past the live grid are captured into
// the compact-line ring (§9.2) by the pre-scroll/eviction HOOK (the LineEvict Callbacks entry
// wired in buildEmu), which x/vt fires synchronously DURING this emu.Write — once per evicted
// line, BEFORE the line is dropped. There is therefore NO post-Write drain pass and NO
// retained-window capture ceiling: a chunk that scrolls thousands of lines (a flood, or a
// `CSI Ps S`) still hands every line to the hook in order. The hook's handler (drainEvictedLine)
// is INTERNALLY GUARDED `if m.shadow.altScreen { return }` (§9.2 "Alt-screen drain guard"): the
// compact ring is the AUTHORITATIVE PRIMARY scrollback only — alt-buffer rows displaced by a
// scroll or a height-shrink MUST NOT be encoded into it (the alt buffer has no scrollback), or
// they would permanently pollute the primary history serialized after the app exits. Because the
// hook fires mid-Write in stream order, m.shadow.altScreen is already correct (the AltScreen
// callback set it earlier in the same Write) when the guard is checked.
//
// RECOVERY (the substantive panic contract — §8.2/§14). x/vt is an untagged ~2-week-old
// emulator fed arbitrary/adversarial PTY bytes, and it buffers PARTIAL escape sequences and
// parser state ACROSS Write calls. A panic mid-parse can therefore leave the emulator's
// internal parser/cursor state corrupted such that EVERY subsequent Write mis-parses — the
// "one chunk behind, reconciled by the next repaint" story is NOT safe and is rejected. So
// on a panic we do NOT continue on the possibly-corrupted emulator: we RECREATE the emulator
// to a known-blank parser state at the CURRENT cols×rows, guaranteeing every later Write
// parses correctly. The visible grid is lost (blank) — but the running app repaints on its
// next frame (TUIs on the implicit redraw, shells re-emit the prompt) and live clients
// already received the raw bytes verbatim before the model was ever touched (NG1, §8.2). The
// compact scrollback ring (m.scrollback / m.sbBytes) is the adapter's OWN immutable bytes,
// independent of x/vt's parser, so it is PRESERVED across the recreate (history is not lost).
func (m *vtModel) Write(p []byte) {
	defer func() {
		if r := recover(); r != nil {
			c, rr := m.emu.Width(), m.emu.Height() // capture size before discarding
			m.recreateEmu(c, rr)                   // fresh *vt.Emulator + shadow.resetTransientModes()
			m.degraded = true                      // sticky; read by Stats via the optional ModelHealth assert (§4)
			m.parsePanics++                        // metric; logged once at warn, not per-chunk
			m.pendingPartial = nil                 // §8.3: a recreated parser is in ground state, no in-flight bytes
		}
	}()
	m.emu.Write(p)            // no IO, no query replies; the LineEvict hook fires here, per
	                          // evicted line, capturing each into m.scrollback / m.sbBytes (§9.2)
	m.trackPendingPartial(p)  // §8.3: update the in-flight partial-sequence buffer for the attach re-sync
}

// drainEvictedLine is the LineEvict (pre-scroll/eviction) HOOK HANDLER — the SOLE
// scrollback-capture path (§9.2, blocker resolution). x/vt invokes it synchronously, once per
// physical line that scrolls off the top of the PRIMARY grid, during emu.Write / emu.Resize,
// handing the leaving row's uv.Cells and its soft-wrap bit BEFORE x/vt drops the line. Because it
// fires per line (not as a post-Write diff), there is NO per-Write capture ceiling: a Write that
// scrolls thousands of lines delivers every one, in order. It is GUARDED twice:
//   (1) `if m.shadow.altScreen { return }`           — alt buffer has no scrollback (§9.2 guard);
//   (2) `if m.shadow.scrollRegionSet && !(m.shadow.scrollTop==1 && m.shadow.scrollBottom==m.emu.Height()) { return }`
//                                                     — a partial-region scroll discards the line.
// Past the guards it encodes the row ONCE via encodeRow(cells, wrapped) (§9.2.1) and appends the
// (bytes, wrapped) pair to m.scrollback, maintaining m.sbBytes O(1). (If the pinned x/vt commit's
// shrink path does not fire the hook, a thin wrapper around emu.Resize routes the displaced rows
// here — §10(a)/§15; if NO source delivers evicted rows, adoption blocks → midterm.)
func (m *vtModel) drainEvictedLine(cells []uv.Cell, wrapped bool)

// trackPendingPartial maintains m.pendingPartial — the raw bytes of the incomplete escape/
// control sequence (if any) at the END of the cumulative stream — for the §8.3 mid-sequence
// attach re-sync. PREFERRED: if the pinned x/vt commit exposes its own parser-ground-state
// predicate / partial buffer (a §15 P0 question), mirror it directly. FALLBACK (always
// available): a minimal ECMA-48 escape-FRAMING state machine carried across Write calls — it
// recognises only sequence FRAMING, never semantics: ground → ESC → CSI (`ESC [` … final
// 0x40–0x7E) / OSC (`ESC ]` … ST `ESC \` or BEL) / DCS·SOS·PM·APC (`ESC P/X/^/_` … ST) / 2-byte
// `ESC` intermediates, plus the 8-bit C1 introducers (0x9B CSI, 0x9D OSC, 0x90/0x98/0x9E/0x9F),
// with CAN 0x18 / SUB 0x1A aborting to ground. Seeded with the prior chunk's carry state, it
// finds where the last COMPLETE sequence ended; the trailing bytes of an in-flight sequence
// become m.pendingPartial (nil in ground state). Capped at maxPendingPartial — past the cap it
// sets pendingPartial = nil (the accepted §8.3 residual). It mutates only m.pendingPartial and
// the carried framing state; it never touches the grid (x/vt owns the real parse).
func (m *vtModel) trackPendingPartial(p []byte)

// PendingInput returns m.pendingPartial (the §8.3 attach re-sync bytes), or nil in ground
// state. Pure read; the session calls it under s.mu in Attach only (never in Snapshot).
func (m *vtModel) PendingInput() []byte { return m.pendingPartial }

// recreateEmu replaces the emulator with a fresh, known-blank one at cols×rows after a
// recovered parse panic, and resets the TRANSIENT shadow modes (alt/mouse/charset/origin)
// to hardware defaults — so x/vt and the shadow agree on a clean primary blank screen and
// the next Write parses from a known-good state. Title and the compact scrollback ring are
// intentionally PRESERVED (they are not parser state). The model↔PTY size invariant (§4.2)
// holds because we rebuild at the captured current size.
func (m *vtModel) recreateEmu(cols, rows int) {
	m.emu = m.buildEmu(cols, rows)
	m.shadow.resetTransientModes() // §7: clears alt/mouse/origin/charset; keeps title; autowrap→ON
}

// Degraded and ParsePanics implement the OPTIONAL model.ModelHealth interface (§4), the
// sanctioned (non-downcast) path by which Stats observes a persistently-degraded session.
// Degraded reports whether this model recovered ≥1 parse panic (screen blanked-and-reparsed).
// Neither changes the TerminalModel contract — Write/Serialize remain fully usable.
func (m *vtModel) Degraded() bool    { return m.degraded }
func (m *vtModel) ParsePanics() int  { return m.parsePanics }
func (m *vtModel) Resize(c, r int) {
	m.emu.Resize(c, r)
	// No wrap side array to reshape (blocker resolution, §9.2): every evicted/visible row's
	// `wrapped` flag is read directly from x/vt's per-row wrap signal at drain/paint time, so a
	// width change carries no stale per-row flag forward — x/vt re-derives wrap state for the
	// post-resize grid itself (the §15 width-reflow question is now an x/vt-internal concern, not
	// ours to reconstruct). The drain below simply re-reads x/vt at the new width.
	// A height-SHRINK pushes the now-overflowing top grid rows off the top, exactly like a
	// scroll — so x/vt fires the SAME LineEvict hook for them DURING emu.Resize, capturing each
	// displaced row into the authoritative compact ring (correct count + sbBytes). There is no
	// post-resize diff pass and no retained window for a row to "age past" (x/vt is
	// WithScrollback(0)); the hook captures the shrink-displaced rows synchronously, in order, as
	// emu.Resize pushes them. The hook handler is INTERNALLY GUARDED on `!shadow.altScreen` (§9.2
	// "Alt-screen drain guard"): a shrink while a full-screen app owns the alt buffer displaces
	// ALT rows, which must NOT enter the primary compact ring. A height-GROW does NOT reflow or
	// pull stored lines back (§10(a)): there is no retained window to repopulate the grid from, so
	// grow simply adds blank rows at the bottom (tmux semantics), and the hook does not fire for a
	// grow. No intervening model.Write can run (Resize holds s.mu, §4.2/§8.3), so the hook sees a
	// consistent shadow state. (§15 P0 confirms the hook fires for shrink-displaced rows; if a
	// pinned commit's shrink path bypasses the eviction hook, a thin wrapper around emu.Resize
	// re-routes the displaced rows through drainEvictedLine, else adoption blocks → midterm.)
}

// (There is intentionally NO reshapeGridWrap / wrap side array — blocker resolution, §9.2: the
// per-row `wrapped` flag is sourced live from x/vt's per-row wrap signal at drain/paint time, a
// HARD P0 adoption gate, so no side array survives a resize to go stale.)
func (m *vtModel) Title() string   { return m.shadow.title }
func (m *vtModel) Cols() int        { return m.emu.Width() }
func (m *vtModel) Rows() int        { return m.emu.Height() }

// HeaderState sources all four §12 header fields from the adapter's own state: size from
// the emulator, `alt` from emu.IsAltScreen() (the physically-painted buffer — the §4.2
// single-source-of-truth value, the same one Serialize uses for steps 3/5/7, so the header
// `<alt>` can never disagree with the `?1049h` in the body), and `scrollbackLines` from the
// compact ring's configured cap (§9.2.2). Pure read, no mutation.
func (m *vtModel) HeaderState() (cols, rows int, alt bool, scrollbackLines int) {
	return m.emu.Width(), m.emu.Height(), m.emu.IsAltScreen(), m.scrollback.Cap()
}

func (m *vtModel) Close()           { /* emu has no IO; reset shadow */ m.shadow = newShadowState() }
```

> **Adapter contract note.** The `vt.Callbacks` field names/signatures above are the
> intended mapping; the spike (§15 P0) pins them to the exact pinned-commit API and
> records any rename in this file only. The interface the session sees does not change.
> The spike MUST resolve these specific x/vt capability questions, each with a committed
> fallback so the design does not depend on an unverified hook:
> - **OSC 7 working-directory callback** — fallback: the existing `parseLastOSC7` scan
>   (kept, §11) feeding `shadow.workingDir`.
> - **Pre-scroll / line-eviction hook (drives scrollback capture) — REQUIRED, NO software
>   fallback (HARD P0 adoption gate, §15 item 7; blocker resolution).** x/vt MUST deliver each
>   line's cells + soft-wrap bit at the instant it scrolls off the primary grid (a `Callbacks`
>   entry, or a thin wrapper around its scroll path), and x/vt is built `WithScrollback(0)`. The
>   previously-listed `ScrollbackLen()`-diff fallback is **DELETED as unsound**: it captures at
>   most `vtScrollbackWindow` (one window) of the lines evicted per `Write`, so a single large
>   `Write` — the RESTORE redraw replaying the full saved depth in one `model.Write`, a
>   multi-screen flood chunk, or a tiny `CSI Ps S` — would silently lose the earlier evictions and
>   punch holes in the authoritative compact ring (§9.2 "Drain mechanism"). No cheap per-`Write`
>   batching bound exists (a 1-byte LF or a few-byte scroll sequence can each evict unboundedly
>   many lines), so the hook is the only correct mechanism. If the pinned commit exposes no such
>   hook and no thin wrapper can synthesise one, x/vt is NOT adopted → midterm (§5.4). `vtScrollbackWindow`
>   is therefore 0 and the §9.3 budget has a SINGLE outcome (retained cell window ~0), not two.
> - **Charset (SCS G0/G1) callback** — fallback: the adapter scans `ESC ( x` / `ESC ) x`
>   in `Write` itself and records the designator in `shadow.g0/g1`.
> - **`Render()` and OSC 8 hyperlinks** — confirm whether `Render()` re-emits the OSC 8
>   wrapper. If yes, hyperlinks survive for free; if no, they are accepted un-restored
>   (§6.2), asserted by the §13.2 hyperlink fixture.
> - **Default colors — REQUIRES a has-been-set signal, not a bare getter.** The getters
>   `Foreground()` / `Background()` / `CursorColor()` return a *resolved* color with no
>   way to distinguish "the app explicitly issued `OSC 10/11/12`" from "x/vt's built-in
>   default." Emitting step 2 from a bare getter would force e.g. white-on-black over a
>   user's light theme on **every** attach. So we require an explicit set-signal:
>   **preferred** — an `OSC 10/11/12` callback (`Callbacks.DefaultColor(slot, color)` or
>   equivalent) that sets `shadow.fgSet/bgSet/cursorColorSet` **and** stashes the color in
>   `shadow.fg/bg/cursorColor`; **fallback if no such callback and no unset sentinel** —
>   the adapter scans `OSC 10/11/12` in `Write` itself (same pattern as the charset/OSC 7
>   scans) and sets the flags + colors; **last-resort fallback** — **omit step 2 entirely**
>   (never emit `OSC 10/11/12`), accepting that an app that recolored its default
>   foreground/background loses that on re-attach and repaints on its next frame, rather
>   than ever overriding the client theme. Step 2 is emitted **only** when the matching
>   `*Set` flag is true. The spike MUST confirm which of these three applies and record it.
> - **Pending-wrap (deferred-autowrap) latch** — confirm whether x/vt exposes the cursor's
>   pending-wrap state (cursor logically one past the last column, autowrap not yet
>   executed). If it does, step 13 reproduces it via the `SerializeAddon` last-column
>   repaint trick (§6.2); if it does not, pending-wrap is accepted un-restored with the
>   bounded residual documented in §6.2 and asserted by the §13.2 `widewrap` fixture.
> The interface the session sees does not change regardless of which fallbacks are used.

The `midterm` swap implements the same two interfaces in a sibling file
(`midterm_model.go`); `midterm` exposes a full grid and a documented serialize path,
so its adapter is strictly simpler (no shadow needed for title/modes if its API is
on-demand). Selection is a build-time constructor choice in one place (§5.3).

### 4.2 Birth size and the model↔PTY size invariant

**INVARIANT (load-bearing): `model size == PTY size` at all times.** No chunk is ever
parsed into the model at a grid size different from the one the PTY (and therefore the
running app) believes it is rendering at. The old behavior — the PTY runs at its kernel
default until the client's first `resize` message, so early output is parsed at an
unknown size — is eliminated, because the model is born at the *same explicit size the
PTY is born at*.

- **Explicit birth size (create path) — net-new cross-stack plumbing.** Live sessions
  are created with an explicit initial size. `Engine.Create` and `session.New` gain
  `initialCols, initialRows int`. **This does not exist today and we build it** — today
  `createTerminal` is `() => Promise<string>` (no args), the `Create` RPC carries only a
  `*domain.TerminalProfile` with no size, and `terminal.tsx` explicitly resizes *after*
  attach once `xterm` is fitted, so the PTY+model are currently born at 80×24 and the
  prompt/startup output is parsed at 80×24 — exactly the corruption window this invariant
  exists to close. The full create-time size plumbing we add:
  1. **FE fits before create.** `terminal.tsx` runs `FitAddon` to measure `xterm`'s
     `cols × rows` **before** calling `createTerminal`. (The terminal pane is laid out at
     mount, so dims are available; if they are not yet measurable, the FE passes the
     80×24 default below rather than guessing.)
  2. **`createTerminal` accepts dims.** Signature changes from `() => Promise<string>`
     to `(cols: number, rows: number) => Promise<string>`
     (`resolve-terminal-connection.ts`, and the create call site in `terminal.tsx`).
  3. **Dims + scrollback thread through the create request.** `cols`/`rows` and
     `scrollbackLines` ride the **transient create-request DTO** (§9.1) — *not* the
     persisted `TerminalProfile` row — through the create RPC → usecase → `Engine.Create`
     → `session.New`.
  4. **PTY sized before the first `Read`.** `session.New` calls
     `pty.Setsize(initialCols, initialRows)` immediately after `pty.Start` and **before**
     the read pump issues its first `Read`, and constructs the model at the same
     `initialCols × initialRows`. Because the PTY is sized before the first read, the very
     first chunk the app emits is already at `initialCols × initialRows` — there is no
     "pre-resize prologue at the kernel default" window to corrupt wrap/scroll.
- **Explicit default.** When the FE supplies no size (the not-yet-measurable case above,
  or a headless/test create), the default is **80 × 24** (the historical PTY kernel
  default), applied **identically** to both PTY and model so they still match.
- **Resize stays in lockstep.** `Session.Resize(cols, rows)` sets the PTY size
  (`pty.Setsize` → `SIGWINCH`) and the model size (`s.model.Resize`) under the **same
  `s.mu` hold, in that order, with no intervening `model.Write`** (the pump cannot run
  between them because it also takes `s.mu`). A chunk is therefore always parsed at the
  PTY's current size. The `s.model.Resize` call is routed through
  `s.mutateModelLocked(func(){ s.model.Resize(cols, rows) })` (§8.5) so a `drainEvictedLine`
  (eviction-hook) panic during the client-driven resize is recovered (bumps `modelPanics`,
  session keeps serving) instead of escaping up `Session.Resize` to crash the readPump/WS goroutine.
- **Resize marks the session dirty and invalidates the cadence cache (load-bearing for
  persistence — §8.4 Issue).** A client resize changes the grid dimensions (and reshapes/
  evicts rows) but produces **no PTY output**, so without an explicit dirty mark `s.dirty`
  would stay false, the §8.4 `Snapshot()` cache would return the **pre-resize** blob with
  `changed == false`, the cadence flush would skip the disk write, and the persisted
  `CRWB1` header would retain the **old `cols×rows`** — so a daemon-restart restore would
  rebuild the model at the stale size and `Setsize` the PTY to the stale size, reintroducing
  the exact wrong-size-restore wrap corruption this invariant exists to eliminate. Therefore
  `Session.Resize`, **in the same `s.mu` hold** that calls `pty.Setsize` + `s.model.Resize`,
  also sets `s.dirty = true` **and** `s.lastBlob = nil` (cache invalidation), so the next
  `Snapshot()` re-serializes at the new size and writes a new-size header. Resize is thus a
  **fourth `s.dirty` site** alongside `pumpStep` (§8.2), `checkForegroundResetLocked`
  (§11.1), and `InjectLocal` (§12). `TestRegression_ResizeOnlyPersistsNewSize` (§13.3)
  guards it.
- **Restore size comes from the `.buf` header, NEVER from FE dims.** On restore the new
  client's size generally differs from the persisted size, and the §6 redraw is
  width-specific and un-reflowed (§10(a)), so the **persisted header size is the single
  authoritative restore size** — FE dims must not win or wrap corrupts. `NewRestored`
  therefore does **not** take `initialCols/initialRows`; it takes the **raw blob**, and
  the shared `spawn()` helper **parses the §12 header and splits off the redraw** (the one
  authoritative parse point — §12), builds the model at the header's `cols × rows` and
  `scrollbackLines`, then `model.Write(redraw)` before the pump starts. **The redraw's step-5
  scrollback lines scroll up through the grid during this single `model.Write` and are captured
  into the reconstructed `lineRing` by the same eviction hook (§9.2) — with NO per-`Write` ceiling,
  so all `scrollbackLines` saved lines are recovered, not just the most-recent window.** (This is
  exactly the manifestation the deleted `ScrollbackLen()`-diff fallback got wrong: it would have
  reconstructed only ~256 lines from the one giant restore `Write`.)
  **The restored PTY is sized to the header dims, identically to the create path
  (Issue: a silent kernel-default-80×24 PTY under a 120×40 model is the exact corruption
  4.2 exists to eliminate).** `spawn()` calls `pty.Setsize(headerCols, headerRows)`
  **immediately after `pty.Start` and before the read pump issues its first `Read`** — so
  the fresh shell's first output (prompt/startup) is emitted *and* parsed at the persisted
  size, never at the kernel default. This holds **even when the re-attaching client's
  xterm happens to equal the header size** and therefore sends no resize message: without
  this `Setsize` the PTY would otherwise stay at 80×24 forever and every subsequent
  serialize of that session would corrupt; with it, PTY and model are born matched. If the
  new client later attaches at a *different* size, the **first client resize** reshapes
  both PTY and model together (the lockstep rule above); until that resize both run at the
  persisted size, so no chunk is ever parsed at a size mismatch.
- **Placeholders hold ONLY the blob — no `vtModel`.** Suspended sessions
  (`NewPlaceholder` / `LoadPlaceholder`) build **no model**: a placeholder has no PTY,
  parses no bytes, and is **never serialized for attach** (the engine attaches by
  re-reading the `.buf` and spawning a live `NewRestored`). Building a full `vtModel` per
  placeholder would pin ~1.8 MB each (live grid + the compact ring; x/vt's retained cell window
  is 0 under `WithScrollback(0)`) for nothing and defeat the very Phase-3c LRU ceiling that exists *because* placeholders
  accumulate. So a placeholder stores **only the raw serialized blob in the dedicated
  `s.rawBlob []byte` field** (named in §8.4, distinct from the live-session `s.lastBlob`
  cache): `ModelBytes()` returns `len(s.rawBlob)`, and `Snapshot()` returns
  `(s.rawBlob, false)` **verbatim** via the `s.model == nil` fast-path (no model access, no
  re-serialize; `changed == false` so a cadence flush of a placeholder writes nothing, §8.4).
  "Construct a model + `model.Write(redraw)`" applies **only** to `NewRestored` (a live
  PTY), never to a PTY-less placeholder. This reconciles §12, §9.4, and this section.

---

## 5. Library adoption

### 5.1 Exact modules

| Module | Role | Tag status | License |
|---|---|---|---|
| `github.com/charmbracelet/x/vt` | Headless VT emulator (parse PTY bytes → grid/scrollback/cursor/modes). | **Untagged** v0 pseudo-version (~2 weeks old at time of writing). | MIT |
| `github.com/charmbracelet/ultraviolet` | The grid→ANSI renderer `x/vt` is built on. Same `uv.Cell` type as `x/vt` → **zero conversion** for grid serialization. | **Untagged** v0 pseudo-version. | MIT |
| `github.com/charmbracelet/x/ansi` | Mature, **tagged** ANSI encoder toolkit. Builds the prologue/epilogue (CUP, DECSET/DECRST, DECSTBM, DECSCUSR, DECTCEM, OSC title, SGR reset). | **Tagged** stable release. | MIT |
| `github.com/vito/midterm` | Pre-vetted **drop-in alternative** emulator behind the same interfaces. Adopted only if `x/vt` proves unfit. | Tagged. | Apache-2.0 / MIT (confirm at adoption). |

All are MIT-family — compatible with the repo. No GPL.

### 5.2 Why these

- `x/vt` is a **headless** emulator (no terminal IO, no query auto-reply) — exactly the
  semantics we need (a headless emulator has no client to answer DA/DSR, so query bugs
  vanish structurally). It shares `ultraviolet`'s `uv.Cell`, so `Render()` emits grid
  ANSI with no cell-type conversion.
- `ultraviolet.Render()` (what `x/vt.Render()` calls) emits **grid-only** ANSI:
  SGR run-length-encoded, ending in ground state — **no cursor positioning, no mode
  sets, no OSC title, no alt-screen enter**. That is the perfect *body*; we wrap it with
  the *prologue/epilogue* we build from shadow state. This division is deliberate and
  matches what `xterm.js` `SerializeAddon` does internally.
- `x/ansi` is **tagged and mature**, so the bytes we *emit* (the risky, correctness-
  critical part) come from a stable encoder. The untagged risk is confined to the
  *parser/grid* (`x/vt`/`ultraviolet`), which the conformance harness (§13) pins.

### 5.3 Untagged-commit pinning strategy

- Pin `x/vt` and `ultraviolet` to **exact commit hashes** in `go.mod` (Go records these
  as `v0.0.0-<timestamp>-<commit>` pseudo-versions). **Never** `@latest`, never a
  floating branch.
- Record the chosen commits + the date + the reviewed-at-that-commit API surface in a
  short `model/UPSTREAM.md` so a future `go get -u` cannot silently move them. **The same
  file pins the conformance oracle: the exact `@xterm/xterm` + `@xterm/addon-serialize`
  versions** (with the committed `model/conformance` lockfile, §13.2) so the conformance bar
  does not drift with an `xterm.js` release. An oracle-version bump is gated identically to
  an x/vt pin bump (harness must re-pass).
- `go mod tidy` is run with the pins already present; CI fails if `go.mod`/`go.sum`
  drift (the repo already gates this — see Wave 0 conventions).
- A weekly Dependabot/CI check may *propose* a bump; bumps are only accepted after the
  conformance harness (§13) passes at the new commit. The interface quarantine means a
  bump touches only `model/`.

### 5.4 The swap plan to midterm

The interface is the contract. To swap:

1. Add `github.com/vito/midterm` to `go.mod`.
2. Implement `TerminalModel` + `Serializer` in `model/midterm_model.go` +
   `model/midterm_serializer.go`.
3. Flip the single constructor `model.New(cols, rows, scrollbackLines)` to build the
   `midterm` adapter instead of the `vt` adapter (one line; or a build tag / config
   constant `model.defaultBackend`).
4. Run the conformance harness (§13.2) against `midterm`; it must pass the same
   **buffer-equivalence** fixture diffs (grid + cursor + modes + colors). It does **not**
   have to reproduce the `x/vt` adapter's exact bytes: the §13.1 **golden-byte** tests are
   inherently per-backend (midterm's `x/ansi`-vs-its-own encoder emits different-but-
   equivalent bytes), so each backend keeps its own golden file. Only the cross-backend
   buffer-equivalence diff is the swap gate. No session/engine code changes.

#### 5.4.1 The fallback is interface-shaped but its HARD-GATE capabilities are UNVERIFIED — a P0 sub-task settles it

**Honest scope of "pre-vetted."** `midterm` is pre-vetted only for the part the *interface*
guarantees: it is a tagged, in-production (Dagger TUI) cell-grid emulator with a documented
serialize-to-ANSI path, so the **buffer-equivalence** swap gate (step 4) and the
`TerminalModel`/`Serializer` shape are low-risk. **It is NOT pre-vetted for the three HARD
P0 ADOPTION GATES this design stakes its scrollback architecture on** (the same gates that
can block x/vt — §15 items 6, 7, and the `Render()`-shape item): (a) a pre-scroll /
line-eviction hook delivering each evicted row's cells **plus its soft-wrap bit** at
eviction time **with no per-`Write` ceiling**; (b) a usable **per-visible-row wrap signal**
(`RowWrapped`-equivalent, or a `Render()` that emits wrapped rows continuously); (c) a
**single-row encoder whose output is byte-identical** to its own grid-`Render()` per-row
output and **self-fixpoints through re-parse** (the `.buf` fixpoint primitive); plus the
prerequisite that midterm holds **bounded-depth scrollback at `scrollbackLines`** at all.
The `WithScrollback(0)` + `LineEvict` + compact-`lineRing` mechanism (§9.2) is **x/vt-shaped**
and may not map onto midterm, whose documented strength is serialize-to-ANSI of a *bounded*
grid — it is not established that midterm has deep (10000-line) scrollback, a per-line
eviction hook carrying wrap bits, or a self-fixpointing single-row encoder. **Therefore the
fallback's hard-gate capabilities are asserted-by-architecture, not verified, and this
design does NOT treat the midterm fall-through as a guarantee.**

**P0 sub-task `M` (RUN FIRST WHENEVER A HARD GATE LOOKS LIKE IT WILL FAIL FOR x/vt — and
unconditionally before P1 if any x/vt hard gate is in doubt): vet midterm against the SAME
hard gates.** Before §15's "fall to midterm" branch may be relied upon, run a throwaway
spike against pinned `midterm` (mirroring the x/vt P0 spike) that **verifies or refutes**,
recording each outcome in `model/UPSTREAM.md`:
- **(M1) bounded-depth scrollback** — midterm retains scrollback to a configurable depth at
  least `scrollbackLines` (default 10000), or exposes a scroll/eviction path we can bound
  ourselves; if it instead keeps an unbounded grid or no scrollback, that is recorded as a
  refutation.
- **(M2) per-line eviction hook with wrap bit, no per-`Write` ceiling** — a callback or a
  thin wrapper around midterm's scroll/height-shrink path that delivers each evicted line's
  cells **and** its `wrapped` bit **at eviction time**, capturing **every** line a single
  large multi-screen `Write` scrolls (the §15-item-7 / §13.3-fifth-case ≥2000-line one-shot
  shape), not just the most-recent screenful.
- **(M3) per-visible-row wrap signal** — a `RowWrapped`-equivalent readable on demand, or a
  `Render()` that emits wrapped grid rows continuously (autowrap-carried, no `CR`/`LF`).
- **(M4) single-row encoder byte-identity + self-fixpoint** — midterm's grid serialize can
  be driven one row at a time (or wrapped to be) such that the per-row bytes are
  byte-identical to its full-grid serialize for that row and round-trip stably through a
  fresh midterm re-parse (the §9.2.1 / §15-item-6 fixpoint primitive, per-backend).

**Tertiary plan (committed, not deferred) if midterm refutes any of M1–M4.** We do **not**
leave a hole. In priority order:
1. **Own a minimal scroll/eviction wrapper over the chosen grid emulator.** Both x/vt and
   midterm parse PTY bytes and own the visible grid; the only capability we strictly need
   them to own is *parse + visible-grid + cursor + modes*. If a backend lacks (M1)/(M2),
   we drive it with `WithScrollback(0)`-equivalent (visible grid only) and interpose our
   **own** scroll-detection wrapper: on each `Write` we diff the pre/post visible top region
   the backend is about to overwrite **per scroll event** (hooking the backend's scroll/LF
   path, not a coarse `ScrollbackLen()` end-of-`Write` diff — that is the deleted, unsound
   ≤256/`Write` mechanism), reading each scrolled-off row's cells + the backend's live
   per-row wrap bit and feeding it to `drainEvictedLine` before the backend overwrites it.
   This wrapper is the SAME mechanism §15 already authorises as the "thin wrapper around its
   scroll path" for x/vt; we make it backend-portable so a backend that exposes the visible
   grid + a per-scroll seam (which any cell-grid emulator must, internally) can satisfy
   (M1)/(M2) even without a first-class hook. The wrapper is in `model/`, behind the
   interface, and is gated by the §13.3 fifth (one-shot ≥2000-line) case exactly as the
   native hook is.
2. **For (M3):** if neither a getter nor a continuous-`Render()` exists, the SAME
   per-scroll/per-paint wrapper that reads cells also reads the backend's live wrap bit for
   visible rows at paint time (bounded `rows×cols`, no ceiling) — there is no software
   *re-derivation* of wrap (still forbidden, §9.2), only a live read through the wrapper's
   per-row seam.
3. **For (M4):** if neither x/vt nor midterm exposes a byte-identical self-fixpointing
   single-row encoder, fall to the **already-specified §13.3 fixpoint fallback** — store the
   **verbatim prior compact-line bytes** for the unchanged scrollback region (immutable
   post-eviction, §9.2) so only the **visible grid** must self-fixpoint, and canonicalise the
   grid SGR/colour emission to one deterministic form so the visible-grid fixpoint holds by
   construction. This already exists for x/vt and is backend-agnostic.
4. **Only if 1–3 cannot satisfy the gates for EITHER backend** do we escalate to evaluating
   a third emulator (e.g. `hinshun/vt10x` or a `tcell`/`screen`-derived grid) or owning a
   minimal scrollback-capable grid layer over a pure parser — recorded as an explicit
   follow-up decision in `model/UPSTREAM.md`, NOT silently assumed. P1 does not start until
   one backend (x/vt via its native hooks, or x/vt/midterm via the portable wrapper of
   plan 1–3) passes all of §15's hard gates.

So the contingency is **architecturally interface-shaped and now has a committed capability
path (sub-task M + the tertiary plan)**, but until sub-task M runs, the midterm fall-through
is an **unverified** contingency, presented as such — not as a guarantee.

---

## 6. Serialize-on-attach byte sequence (exact order)

`Serialize(m)` emits the following bytes, **in this exact order**, into a single
`[]byte`. Primitives are from `x/ansi` unless noted. The order is a deliberate
**refinement of the originally-sketched locked order** so it matches the `xterm.js`
`SerializeAddon` oracle (§6.1): **all content is painted into a clean default baseline
(autowrap ON, origin OFF, full-screen margins, default charset) FIRST, and every mode /
margin / origin / cursor that would distort that paint is asserted AFTER the paint.**
The result ends in ground state and is, by construction, self-contained (no dangling
sequence, no query, no historical OSC).

Two cursor facts drive the order, and must be respected exactly: **(i)** `Render()` is
**positionless** — it paints the grid rows starting at the *current* cursor cell,
advancing row-to-row via `CR`/`LF` for **hard** line breaks (no absolute moves) while
emitting a **soft-wrapped** row's content continuously with **no** `CR`/`LF` (autowrap, forced
ON in step 1, carries the wrap so wrap state and physical line-count survive), and emits
**NO trailing newline** after the final grid row (so the grid never scrolls itself).
**This `Render()` shape is load-bearing for steps 5–7 and is empirically verified at the
pinned commit by P0 exit-gate item #5 (§15) — on a fixture WITH scrollback, not the
scrollback-free vim/alt fixture — with a committed fallback (adapter post-process **α** or the
fully-specified own-per-row-encoder grid paint **β**, both byte-level specified in §15) if it
does not hold; it is not assumed unverified.**
Consequently the **scrollback (step 5) and the visible grid (step 7) are ONE continuous
stream** — all stored scrollback rows first, the `rows` grid rows last, joined by `CR`/`LF`
at hard line breaks and by nothing (autowrap) between a soft-wrapped row and its continuation —
exactly as `xterm.js` `SerializeAddon` emits a screen. The earlier lines scroll up into
xterm's history and the **trailing `rows` lines are precisely the visible grid**. There is
therefore **NO re-home between scrollback and the grid** (the old step-6 re-home is deleted,
see step 6): a re-home would strand the most-recent `rows` scrollback lines on the visible
region, where step 7's grid overwrites them — **permanently losing the newest, most
important history**. When scrollback is empty (or alt-screen, step 5 skipped) the cursor is
already at `(1,1)` from step 4, so the grid paints from home with no scrolling.
**(ii)** `DECSTBM` (step 11) homes the
cursor, and **origin mode `DECOM ?6` (step 12) BOTH re-homes the cursor to the scroll-region
top-left AND changes how `CUP` coordinates are interpreted**: with `?6h` active xterm/xterm.js
move the cursor to region-home the instant the mode is processed (`InputHandler` `setMode`
case 6 calls `_setCursor` to region top-left), and a subsequent `CUP` is interpreted
*relative to the scroll region*. Therefore `?6h` MUST be asserted **before** the final cursor
`CUP` (step 13), **never after it** — emitting `?6h` after an absolute `CUP` would drag the
cursor back to region-home and destroy the step-13 placement (this is the exact ordering bug
this sequence fixes). When origin mode is active, the step-13 `CUP` is emitted in
**region-relative coordinates** (`row = absRow − (scrollTop − 1)`, `col` unchanged — we track
no left/right margins); when origin mode is OFF it is absolute. This matches how
`SerializeAddon` sequences origin mode relative to the cursor restore. The
**"Cursor after"** column states the invariant the next step relies on.

| # | Step | Condition | Bytes / primitive | Cursor after |
|---|---|---|---|---|
| 1 | **DECSTR soft reset, then EXPLICIT autowrap-ON.** DECSTR (`ESC [ ! p`) establishes a known baseline (origin OFF, full-screen margins, default SGR, default G0/G1 charset). We then emit `ESC [ ? 7 h` to **force DECAWM (autowrap) ON deterministically** — DECSTR's effect on DECAWM is *implementation-defined* and several emulators (including some xterm.js versions) do **not** force it ON via soft reset, so we do not rely on it; the explicit `?7h` makes the autowrap-ON baseline correct regardless (P0 §15 records the pinned-commit DECSTR behavior, but correctness no longer depends on it). This autowrap-ON baseline is load-bearing for the wrapped-row paint in steps 5/7. (`ESC c` RIS is too heavy.) | always | `ESC [ ! p` → `ansi.SoftReset`, then `ESC [ ? 7 h` → `ansi.SetMode(ansi.DECMode(7))` | (1,1), autowrap ON |
| 2 | **Default colors.** Restore app-changed default fg/bg/cursor color so default-attribute cells and the cursor render correctly under the painted grid. **Emitted from the shadow set-flags, NOT a bare getter** — see note below; a bare getter cannot tell "app set it" from "x/vt default" and would override the client theme on every attach. | each only if its shadow `*Set` flag is true (`shadow.fgSet` / `bgSet` / `cursorColorSet`), i.e. the app explicitly issued the matching `OSC 10/11/12` | `OSC 10 ; <fg> ST` / `OSC 11 ; <bg> ST` / `OSC 12 ; <cursor> ST` (`ansi.SetForegroundColor` / `SetBackgroundColor` / `SetCursorColor`) from `shadow.fg/bg/cursorColor` | (1,1) |
| 3 | **Alt-screen enter.** Switch to the alternate buffer *before* painting it. | only if `shadow.altScreen` | `ESC [ ? 1049 h` → `ansi.SetMode(ansi.DECMode(1049))` | (1,1), alt buffer |
| 4 | **Clear + home.** | always | `ESC [ 2 J` (`ansi.EraseDisplay(2)`) then `ESC [ H` (`CursorPosition(1,1)`) | (1,1) |
| 5 | **Scrollback lines (WRAP-AWARE).** Emit each compact stored row in chronological order. **After a row whose stored `wrapped` flag is FALSE (a logical line that ended at a hard break) emit `\r\n`; after a row whose `wrapped` flag is TRUE (it soft-wrapped into the next stored row) emit NOTHING** — the continuation row follows immediately and autowrap (forced ON in step 1) carries it, so the reconstructed buffer keeps the **identical wrap state and physical line-count** as the model and as `SerializeAddon` (which likewise emits no separator between a wrapped row and its continuation). The last stored row is followed by `\r\n` **iff it is not wrapped**, so the grid (step 7) begins on a fresh line and steps 5+7 form ONE continuous flow. Because the flow's trailing `rows` physical rows are the grid, **every** stored row scrolls into xterm's history — none is left on the visible region to be overwritten by `Render`. A **wrapped** row's compact bytes are encoded at **full `cols` width with NO trailing-blank trimming** (so autowrap fires at exactly the right column); a **non-wrapped** row is trimmed (the §9.2.1 per-row encoder). | **only if `!shadow.altScreen`** — the alt buffer has no scrollback; emitting `\r\n` lines into it would scroll the alt grid and pollute the primary buffer xterm restores on `?1049l` (§10(b)). | per-row compact bytes (§9.2.1) + `\r\n` only when `!wrapped` | bottom row, col 1 |
| 6 | **No re-home (deleted step).** The grid paint (step 7) MUST continue **directly** from the step-5 flow — do **NOT** return the cursor to `(1,1)` here. A re-home would leave the most-recent `rows` scrollback lines on the visible grid, where step 7 overwrites them, permanently losing the newest history (the §13 round-trip scrollback-depth assertion guards this). With no re-home, scrollback + grid are one continuous flow whose trailing `rows` lines are the visible grid and whose earlier lines all land in xterm history. | always | *(no bytes emitted)* | unchanged — bottom row after step 5, or `(1,1)` when step 5 was empty/skipped |
| 7 | **Visible grid (wrap-aware, EXACTLY `rows` physical rows).** Grid-only ANSI (SGR run-length, **no** cursor moves / modes / OSC / alt-enter): hard line breaks advance row-to-row via `CR`/`LF`, while a **soft-wrapped** grid row emits its content continuously with **NO** `CR`/`LF` (autowrap, forced ON in step 1, carries the wrap — exactly as the oracle does), and there is **no trailing newline after the final row**. **The grid body MUST emit exactly `rows` physical rows — trailing blank grid rows included, each as a bare `CR`/`LF` line (an empty physical line), NEVER trimmed away.** Within a row, trailing blank *cells* are still trimmed (a non-wrapped row, §9.2.1), but a wholly-blank row still contributes its line break so the physical-row count is precisely `rows`. **This is load-bearing and is the §13.3 `RoundTripScrollbackDepth` invariant (Issue):** because the scrollback flow (step 5) and the grid are ONE continuous stream, the flow must have exactly `sb_count + rows` physical lines so that **all** `sb_count` scrollback rows scroll into xterm history and the trailing `rows` lines land in the visible viewport. If the grid body emitted fewer than `rows` rows (trailing blank rows trimmed), the flow would be short, the bottom-most scrollback lines would **strand in the viewport instead of history**, and the newest history would be lost — the exact bug step 6's "no re-home" already guards against, re-introduced through row-trimming. In the **β** branch this holds by construction: the per-row encoder (§9.2.1) walks all `rows` grid rows, emitting a blank row as a bare line. In the **α** branch (default) this body is `m.Render()`; **P0 exit-gate item 5 (§15) empirically verifies — on a fixture WITH scrollback AND the cursor parked ABOVE the bottom row (so the grid has trailing blank rows) — that `Render()` emits exactly `rows` physical rows.** If `Render()` trims trailing blank rows (as `SerializeAddon` itself does), the α post-process **MUST re-pad the body back to exactly `rows` rows** by appending the missing bare `CR`/`LF` line breaks before it becomes step 7's body, or — if α cannot reliably re-pad — **fall to β** (which is exact by construction). It continues from the step-5 cursor (or `(1,1)` when scrollback was empty/skipped), so its `rows` rows become the trailing — visible — lines of the flow. | always | unspecified (set by step 13) |
| 8 | **DEC private modes, default-OFF → SET-if-on / omit-if-off**, ascending order: `?1` DECCKM, `?1000/?1002/?1003` mouse, `?1006/?1015` mouse-encoding, `?1004` focus, `?2004` bracketed paste. (`?1049` is step 3; `?6`/`?7` are special, below.) | per mode if `shadow.modes[n] == true` | `ansi.SetMode(ansi.DECMode(n))` → `ESC [ ? n h` | unchanged |
| 9 | **Autowrap `?7`, default-ON → RESET-if-off / omit-if-on.** Emit `?7l` *only if the app disabled autowrap*; otherwise omit (leaving the baseline ON). Emitted **after** the grid paint, which required autowrap ON. | only if `shadow.modes[7] == false` | `ansi.ResetMode(ansi.DECMode(7))` → `ESC [ ? 7 l` | unchanged |
| 10 | **Charset designation (SCS) + active locking-shift.** Re-designate G0/G1 **and** re-invoke whichever is mapped into GL, so the *live stream after re-attach* renders line-drawing/alt charset correctly. Grid cells already hold resolved runes, so this affects only subsequent app output. The designation alone is insufficient: a DEC line-drawing TUI invokes G1 via `SO`, so without re-emitting the locking-shift the post-attach stream is parsed as GL=G0 while the app believes GL=G1 and mis-renders. | designation: per slot if `shadow.g0 != 'B'` / `shadow.g1 != 'B'`. shift: emit `SO` if `shadow.glLock == 1`; emit `SI` if `shadow.glLock == 0` **and** a G1 designation was emitted (re-assert GL=G0 baseline); else omit (baseline GL=G0). | `ESC ( <g0>` and/or `ESC ) <g1>`, then `SO` (0x0E) or `SI` (0x0F) | unchanged |
| 11 | **Scroll region (DECSTBM).** Emitted **after all content** (oracle sets margins LAST); DECSTBM homes the cursor, hence before origin mode and the final CUP. | only if `shadow.scrollRegionSet` and not full-screen | `ESC [ <top> ; <bottom> r` → `ansi.SetTopBottomMargins(top,bottom)` | region top-left (or (1,1) if no region) |
| 12 | **Origin mode `?6` — asserted BEFORE the cursor CUP.** Re-assert DECOM **before** step 13 so the final `CUP` is interpreted in the coordinate space the app expects. Enabling `?6h` itself re-homes the cursor to the scroll-region top-left (xterm/xterm.js move the cursor on `setMode 6`); emitting it *after* an absolute `CUP` would drag the cursor back to region-home and destroy the placement — **the exact bug this order fixes**. So origin mode is set here, and step 13 then issues a region-relative `CUP`. | only if `shadow.modes[6] == true` | `ansi.SetMode(ansi.DECMode(6))` → `ESC [ ? 6 h` | region top-left |
| 13 | **Cursor position (CUP) + pending-wrap re-arm.** To the real cursor cell from `CursorPosition()`. **Coordinates depend on origin mode (step 12):** when `shadow.modes[6]` is **OFF**, emit an **absolute** `CUP` (`row`, `col`, 1-based); when origin mode is **ON**, emit a **region-relative** `CUP` — `row = absRow − (top − 1)` where `top = scrollRegionSet ? scrollTop : 1`, `col` unchanged (no left/right margins tracked) — because with `?6h` active xterm interprets `CUP` relative to the region (the real cursor is always inside the region when origin mode is active, so `row ≥ 1`). **Pending-wrap (deferred-autowrap) latch:** if the cursor is logically one past the last column with autowrap not yet executed, the bare `CUP` clears that latch in the target xterm; when x/vt exposes the latch (P0 spike, §15) we reproduce it with the `SerializeAddon` last-column repaint trick (`CUP` to the **last** column, **re-print the last cell's glyph with its SGR**, re-arming pending-wrap, leaving the cursor logically past the margin); when x/vt does not expose it, pending-wrap is **accepted un-restored** (§6.2). The last-column repaint also uses region-relative coordinates when origin mode is ON. | always | `ESC [ <row> ; <col> H` → `ansi.CursorPosition(col,row)` (region-relative iff origin mode ON); plus the last-column repaint when the latch is set and exposed | real cursor (pending-wrap re-armed if applicable) |
| 14 | **Cursor shape (DECSCUSR).** | **only if `shadow.cursorShapeSet`** — the app explicitly issued DECSCUSR; otherwise omitted so the client's user-configured default cursor is preserved (see note). | `ESC [ <n> SP q`, `n` per the mapping below | real cursor |
| 15 | **Cursor visibility (DECTCEM).** | always (one deterministic byte either way) | `ESC [ ? 25 h` if visible else `ESC [ ? 25 l` | real cursor |
| 16 | **Title.** Deterministic OSC selection (note below). **`title` and `iconName` are SANITIZED + BOUNDED via `sanitizeOSCText` (below) BEFORE emission** — this is mandatory, not optional: the stored text is app-controlled and §1.2#2 documents apps embedding raw ANSI/ESC inside the OSC title parameter. `ST`-terminated, never bare BEL. **This is the one deliberate deviation from the oracle** (§6.1). | `OSC 1` only if `sanitize(iconName) != ""`; `OSC 2` only if `sanitize(title) != ""` | `ESC ] 1 ; <sanitize(icon)> ST` then `ESC ] 2 ; <sanitize(title)> ST` (`ansi.SetIconName` / `SetWindowTitle`) | real cursor |
| 17 | **Ground-state SGR reset.** Final bytes; parser ends in ground state with the default pen so the live stream that follows is interpreted cleanly. | always | `ESC [ 0 m` → `ansi.ResetStyle` | real cursor |

**DECSCUSR mapping (deterministic).** `vt.CursorStyle` → DECSCUSR `n`: `block → 1`
(blink) `/ 2` (steady); `underline → 3 / 4`; `bar → 5 / 6`. `shadow.cursorBlink`
selects the odd (blink) vs even (steady) member. Emitted only when `cursorShapeSet`
(step 14), so an app that never set a shape never overrides the client's default cursor.

**OSC 0/1/2 selection (deterministic).** We **never** emit `OSC 0` (it conflates
icon+title and would make the byte stream state-dependent). We emit `OSC 1` (icon) iff
`sanitizeOSCText(iconName) != ""` and `OSC 2` (title) iff `sanitizeOSCText(title) != ""`,
icon before title, each independently. Fully determined by (sanitized) shadow state → the
§13.1 golden-byte tests are stable.

**Title/icon sanitization + bounding (`sanitizeOSCText`, MANDATORY — closes the
re-introduced-dangling-OSC hole).** `shadow.title` / `shadow.iconName` are **app-controlled
text** captured from `OSC 0/1/2`. §1.2#2 itself documents zsh/git prompts embedding full
ANSI-colored sequences (raw `ESC`, `BEL`, C0 bytes) inside the OSC parameter. Re-emitting
that text verbatim inside our own `OSC 2 ; … ST` would let an embedded `ESC`/`ST`/`BEL`
**prematurely terminate or never-terminate** our OSC — re-creating the exact garbled-tab /
stream-swallow class this whole design exists to kill. **We do NOT assume x/vt pre-cleans
the stored title.** The serializer therefore runs every title/iconName through a single
specified function before step 16 emits it:

```go
// sanitizeOSCText makes app-controlled OSC string text safe to re-emit inside our own
// OSC 1/2 ... ST envelope. It is the SOLE path by which title/iconName reach the wire.
//   1. Decode as UTF-8; replace each invalid byte with U+FFFD (no raw bytes pass through).
//   2. DROP every C0 control (0x00–0x1F, which includes ESC 0x1B and BEL 0x07) and DEL
//      (0x7F). This guarantees no embedded ESC, no embedded ST/BEL terminator, no stray
//      C0 can split or run-away our OSC. (We drop rather than space-replace so a colored
//      prompt collapses to its visible text, matching what a tab UI wants to show.)
//   3. Also drop C1 controls (U+0080–U+009F), since 0x9C is the 8-bit ST.
//   4. CAP the result at maxOSCTextRunes (= 256 runes); truncate on a rune boundary.
// The output contains only printable, ground-safe text; emitting it inside
// ESC ] 2 ; <text> ST is therefore always a single, well-formed, ST-terminated OSC that
// cannot swallow the following plain-text write.
func sanitizeOSCText(s string) string

const maxOSCTextRunes = 256
```

The output is **always** terminated with the 7-bit `ST` (`ESC \`), **never** a bare `BEL`,
so the envelope is unambiguous. An empty result after sanitization means the corresponding
OSC is omitted entirely (the `!= ""` gate above operates on the *sanitized* value). This is
asserted by a `model/` unit test and by `TestRegression_ReattachAdversarialTitle` (§13.3),
which sets a title containing an embedded `ESC[…m` and a missing terminator, re-attaches,
and asserts the payload's OSC title is well-formed, single, `ST`-terminated, and does not
swallow a following plain-text write.

**Default-color OSC byte format (canonical, deterministic — step 2).** The byte form of the
`OSC 10/11/12` payload is **pinned**, because the §13.1 golden-byte tests and the §13.3 `.buf`
self-fixpoint (parse our own OSC color back through x/vt, re-serialize, expect byte-identity)
require a single canonical encoding and x/vt's OSC-10/11/12 callback may hand back a *resolved*
color whose round-trip form would otherwise differ from what we emit. The serializer emits the
**canonical xterm 16-bit-per-channel form `rgb:RRRR/GGGG/BBBB`** — each channel four **lowercase**
hex digits scaled to 16 bits — e.g. a pure-white background → `ESC ] 11 ; rgb:ffff/ffff/ffff ST`.
We choose this form over `#RRGGBB` and over the 8-bit `rgb:RR/GG/BB` because it is xterm's
canonical OSC-color form, it round-trips **losslessly** through x/vt's OSC-10/11/12 parser, and
it is what the `SerializeAddon`/xterm oracle accepts — so feeding our own emitted bytes back
into x/vt and re-serializing yields **byte-identical** output. The shadow `color.Color` is
converted via its 16-bit channels (`color.Color.RGBA()` returns pre-multiplied 16-bit values;
we use them directly), formatted as lowercase hex zero-padded to 4 digits per channel, and
**`ST`-terminated, never bare BEL**. This canonical encoding is a determinism requirement on par
with canonical SGR (§13.1) and is asserted to self-fixpoint through x/vt (§13.1, §13.3
`SerializeFixpointThroughXVT`, and the §13.2 `oscolor` fixture).

**Mouse-encoding scope.** Tracked mouse modes are exactly `?1000` (X11), `?1002`
(button-event), `?1003` (any-event), plus encodings `?1006` (SGR) and `?1015` (urxvt).
`?1005` (UTF-8 mouse, deprecated/ambiguous), `?1016` (SGR-pixels), and `?1001`
(highlight-tracking, requires terminal cooperation) are **intentionally out of scope** —
obsolete or interactive-only, unused by our target apps; an app relying on them
re-asserts on its next frame.

**The whole payload is one `OutputFrame`** queued onto the new client's `send` channel
before it joins the live fan-out, identically to how replay was queued — the FE,
transport, and `writePump` are unchanged.

### 6.2 Accepted un-restored state (saved-cursor, palette) and the OSC scope

Origin mode (`?6`), active G0/G1 charset designation **and** the active locking-shift
invocation (`SI`/`SO`, step 10) **are** restored. The following are explicitly **not**
serialized, each with a documented residual:

- **Saved cursor (`DECSC`/`DECRC`, `ESC 7`/`ESC 8`).** x/vt does not surface the
  saved-cursor register, and it is a transient app-internal scratch that essentially
  never needs to survive a redraw boundary. **Residual:** a `DECRC` issued by the app
  immediately after re-attach *without* a preceding `DECSC` restores to `(1,1)` +
  default SGR instead of the app's pre-attach saved cursor. Vanishingly rare (apps pair
  DECSC/DECRC within one render and repaint on the SIGWINCH the attach implies) and
  **matches tmux**, which likewise does not carry the saved-cursor register across
  reattach. Accepted.
- **Saved primary buffer behind an active alt screen (the G3 carve-out).** When the model
  is in the alt buffer at attach, the *primary* buffer it saved (the pre-app shell screen +
  its scrollback) is **not** serialized — x/vt does not surface the inactive primary grid,
  and the locked decision (§10(b)) serializes only the active alt grid + `?1049h`.
  **Residual:** if the alt app exits (`?1049l`) after a re-attach, the new xterm restores an
  **empty** (never corrupted) primary instead of the original pre-app screen; the shell then
  redraws its prompt on its next output. Bounded to the alt-reattach-then-exit corner.
  Asserted by `TestRegression_ReattachDuringAltScreenThenExit` (§13.3). Accepted.
- **Palette (OSC 4).** x/vt resolves indexed colors against its palette into each
  cell's color at write time, and `Render()` emits the resolved color, so static cells
  render correctly even without re-emitting OSC 4. Only an app that changes the palette
  *and then* writes new palette-relative cells after re-attach drifts, and it repaints
  on its next frame. Not serialized. Accepted.
- **OSC 8 hyperlinks.** ultraviolet cells carry hyperlink identity; whether `Render()`
  re-emits the `OSC 8` wrapper is confirmed in the P0 spike (§15). **Decision:** if
  `Render()` emits OSC 8, hyperlinks survive automatically; if not, they are accepted
  un-restored — the cell *text* stays correct, only the clickable-link metadata is lost
  until the app repaints. Either way the §13.2 hyperlink fixture makes the actual
  behavior an asserted, non-regressing fact rather than an accident.
- **Pending-wrap (deferred-autowrap) latch — CONDITIONALLY restored, else bounded
  residual.** When x/vt exposes the latch, step 13 re-arms it (last-column repaint, after
  the step-12 origin-mode assertion) and it is fully restored. When x/vt does **not** expose
  it, it is accepted un-restored:
  **residual** — a printable char written on the live stream *immediately* after
  re-attach, when the pre-attach cursor was parked in the pending-wrap state at the right
  margin, lands at the right margin (overwriting the last cell) instead of wrapping to the
  next row's first cell — a **one-cell, one-write** discrepancy that self-heals on the
  app's next repaint (TUIs repaint on the SIGWINCH the attach implies; shells re-emit the
  prompt). The residual is bounded to that single first post-attach glyph. The §13.2
  `widewrap` fixture asserts which branch is in effect and that the residual is no worse
  than one cell.

### 6.1 Conformance oracle: xterm.js `SerializeAddon`

`xterm.js`'s `SerializeAddon` is the **industry-reference** "serialize current screen to
ANSI" implementation and is our **conformance oracle** (§13). For a given screen state
our serializer's output, fed into a fresh `xterm`, must produce a buffer **identical** to
what `SerializeAddon` would reproduce.

**Known, intentional deviation:** `SerializeAddon` **omits the OSC title** (step 16) —
it serializes the *buffer*, not the *window title*, because in a browser the title is
DOM, not buffer. We **add** the title because our tab UI is driven by it. The conformance
harness therefore diffs **buffer-equivalence** (grid + cursor + modes), and asserts the
title separately. No other deviation in *buffer outcome* is permitted; any other diff is
a bug in our serializer. (Note: the *byte stream* legitimately differs from
`SerializeAddon` because our order is tuned to the corrected sequence above and we add
the title; only the resulting **buffer** must match — see §13.1/§13.2 on golden-byte vs
buffer-equivalence gates.)

---

## 7. Shadow-state struct

`x/vt` does not let us *read back* the push-only state, so the adapter mirrors it.
Every field, the callback that feeds it, and how `Serialize` consumes it:

```go
package model

// shadowState mirrors the screen state x/vt delivers via Callbacks (push-only) and
// does NOT expose via on-demand getters. The vtModel's Callbacks closures are the
// SOLE writers; Serialize is the SOLE reader. Both run under the session's s.mu, so
// shadowState needs no internal lock.
type shadowState struct {
	title         string       // ← Callbacks.Title    → step 16 (OSC 2)
	iconName      string       // ← Callbacks.IconName → step 16 (OSC 1, if non-empty)
	altScreen     bool         // ← Callbacks.AltScreen → step 3 (?1049h when true) + gates step 5
	cursorVisible bool         // ← Callbacks.CursorVisibility → step 15 (DECTCEM)
	cursorShape   vt.CursorStyle // ← Callbacks.CursorStyle → step 14 (DECSCUSR n)
	cursorBlink   bool         // ← Callbacks.CursorStyle → step 14 (blink → DECSCUSR odd/even)
	cursorShapeSet bool        // ← Callbacks.CursorStyle → GATES step 14 (app explicitly set DECSCUSR)
	scrollRegionSet bool       // ← Callbacks.ScrollRegion → gates step 11
	scrollTop     int          // ← Callbacks.ScrollRegion → step 11 (DECSTBM top)
	scrollBottom  int          // ← Callbacks.ScrollRegion → step 11 (DECSTBM bottom)
	g0            byte         // ← Callbacks.Charset/SCS scan → step 10 (default 'B' = US-ASCII)
	g1            byte         // ← Callbacks.Charset/SCS scan → step 10 (default 'B' = US-ASCII)
	glLock        int          // ← Callbacks.LockingShift / SI-SO scan → step 10 (0 = G0/SI, 1 = G1/SO)
	workingDir    string       // ← Callbacks.WorkingDirectory (or parseLastOSC7 fallback) → s.cwd
	modes         map[int]bool // ← Callbacks.EnableMode/DisableMode → steps 3/8/9/12 (DECSET/DECRST)
	fg            color.Color  // ← Callbacks.DefaultColor(0) / OSC 10 scan → step 2 (only if fgSet)
	bg            color.Color  // ← Callbacks.DefaultColor(1) / OSC 11 scan → step 2 (only if bgSet)
	cursorColor   color.Color  // ← Callbacks.DefaultColor(2) / OSC 12 scan → step 2 (only if cursorColorSet)
	fgSet         bool         // ← set true when the app issues OSC 10  → GATES step 2 (fg)
	bgSet         bool         // ← set true when the app issues OSC 11  → GATES step 2 (bg)
	cursorColorSet bool        // ← set true when the app issues OSC 12  → GATES step 2 (cursor color)
}

// newShadowState returns a shadow with terminal HARDWARE defaults: cursor visible, no
// alt screen, no scroll region, empty title, no modes set, G0/G1 = US-ASCII ('B'),
// GL invoking G0 (glLock 0), no app-set default colors (fgSet/bgSet/cursorColorSet
// false), cursor shape NOT explicitly set. Note that hardware defaults differ per mode —
// see setMode — and Serialize uses per-mode default polarity (step 8 vs step 9).
func newShadowState() shadowState {
	return shadowState{cursorVisible: true, g0: 'B', g1: 'B', glLock: 0, modes: make(map[int]bool)}
}

// setMode records a DEC private mode's on/off state. Only modes Serialize re-asserts
// are retained; others are ignored so the map stays small and Serialize is minimal.
// IMPORTANT: the map stores the TRUE on/off state as observed; Serialize applies the
// correct default polarity per mode (default-OFF modes emit SET-if-on in step 8;
// the default-ON mode ?7 emits RESET-if-off in step 9; ?6 is step 12; ?1049/?47/?1047
// drive alt-screen, step 3). The map never assumes a uniform default.
//
// Hardware defaults of the tracked modes (for the polarity split):
//   ?1   DECCKM (app cursor keys) ........ default OFF  → step 8
//   ?6   DECOM (origin mode) ............. default OFF  → step 12 (before the step-13 CUP)
//   ?7   DECAWM (autowrap) .............. default ON   → step 9 (RESET-if-off)
//   ?1000/?1002/?1003 mouse ............. default OFF  → step 8
//   ?1004 focus reporting ............... default OFF  → step 8
//   ?1006/?1015 mouse encoding .......... default OFF  → step 8
//   ?2004 bracketed paste ............... default OFF  → step 8
//   ?47/?1047/?1049 alt-screen .......... default OFF  → step 3 (via shadow.altScreen)
func (s *shadowState) setMode(mode int, on bool) {
	switch mode {
	case 1, 6, 7, 1000, 1002, 1003, 1004, 1006, 1015, 1047, 1049, 2004, 47:
		s.modes[mode] = on
	}
}

// setCharset records a G0/G1 charset designation (SCS). slot 0 = G0 (ESC ( x),
// slot 1 = G1 (ESC ) x); designator is the final byte ('B' US-ASCII, '0' DEC line
// drawing, etc.). Consumed by Serialize step 10.
func (s *shadowState) setCharset(slot int, designator byte) {
	switch slot {
	case 0:
		s.g0 = designator
	case 1:
		s.g1 = designator
	}
}

// setDefaultColor records an app-issued OSC 10/11/12 default color and flags it as
// explicitly set, so Serialize step 2 emits it ONLY for colors the app actually changed
// (a bare getter cannot distinguish app-set from x/vt's built-in default and would
// override the client theme on every attach). slot 0 = fg, 1 = bg, 2 = cursor.
func (s *shadowState) setDefaultColor(slot int, c color.Color) {
	switch slot {
	case 0:
		s.fg, s.fgSet = c, true
	case 1:
		s.bg, s.bgSet = c, true
	case 2:
		s.cursorColor, s.cursorColorSet = c, true
	}
}

// resetTransientModes clears the app-owned, transient screen state to hardware
// defaults WITHOUT touching grid/scrollback/title/cursor. Called on the
// foreground-app-died edge (§11.1) so a SIGKILLed app cannot leave mouse/alt-screen
// modes stuck ON for the next Serialize into an idle shell.
func (s *shadowState) resetTransientModes() {
	s.altScreen = false
	s.cursorShapeSet = false
	s.g0, s.g1 = 'B', 'B'
	s.glLock = 0 // GL returns to invoking G0 (SI)
	for _, m := range []int{1, 6, 1000, 1002, 1003, 1004, 1006, 1015, 1047, 1049, 2004, 47} {
		delete(s.modes, m)
	}
	s.modes[7] = true // autowrap returns to its default-ON state
	s.scrollRegionSet = false
	// Default colors (OSC 10/11/12) are deliberately NOT reset: like a real terminal,
	// app-set default fg/bg/cursor colors persist across a foreground app's death until
	// another app or the user resets them. fgSet/bgSet/cursorColorSet stay as-is.
}
```

`Serialize` consumes the shadow exactly as each §6 step's "Source" / "Condition" notes
state (the per-field arrows above name the step each field feeds). The
`workingDir` field is *not* serialized into the redraw (it is engine metadata); it
updates `s.cwd` via the model, preserving today's OSC 7 → CWD behavior (§11).

---

## 8. Concurrency model

### 8.1 The lock

`x/vt` is **not** thread-safe. We have two options: `vt.SafeEmulator` (its built-in
mutex wrapper) or **our own existing lock**. We use **our own `s.mu`**, because the
session *already* serializes `pumpStep` and `Attach` under `s.mu`, and the model is only
ever touched from those two paths. Wrapping in `SafeEmulator` too would double-lock for
no benefit. (We document this; if a future path needs to touch the model outside `s.mu`,
the `vtEmu` wrapper (§4.1) embeds `*vt.SafeEmulator` instead of `*vt.Emulator` — a one-line,
file-local change behind the `emulator` seam, with the `vtModel.emu emulator` field and
every call site unchanged. This is the documented escape hatch.) **The `emulator` seam
itself adds no lock:** `vtEmu` is a zero-state pass-through, so introducing it for testability
(§4.1) does not weaken or double the locking — `s.mu` is still the sole synchronisation.

Lock order is unchanged and preserved: `sessionMu (engine, outer) → s.mu → (model internal)`.
The old `s.mu → ring.mu` nesting is gone with the ring.

### 8.2 Fan-out-first ordering (why the model never adds latency)

`pumpStep` becomes:

```go
func (s *Session) pumpStep(chunk []byte) {
	path, ok := parseLastOSC7(chunk)   // unchanged, outside the lock (race-free: chunk is owned)
	s.mu.Lock()
	defer s.mu.Unlock()                // ALWAYS release s.mu, even if model.Write panics (see below)
	if ok {
		s.cwd = path
	}
	s.fanOutLocked(chunk) // (1) LIVE FIRST: enqueue raw bytes to every client — non-blocking sends
	s.writeModelLocked(chunk) // (2) THEN model: panic-isolated VT parse (helper below)
	s.dirty = true
	// (3) LAST: debounced foreground-process-group sample (§11.1 sampling site #1). This is
	// the ONLY place the TIOCGPGRP ioctl (and, on the app-death edge, the synthetic teardown
	// model.Write) may run inside pumpStep, and it MUST be here — strictly AFTER fanOutLocked
	// AND writeModelLocked — so neither the ioctl nor the teardown write can ever precede or
	// delay the live fan-out (NG1: zero added latency). The 250 ms lastFgSampleAt gate keeps
	// it off the hot path (≤ ~4 ioctls/s/session). This ordering is a HARD REQUIREMENT,
	// asserted by TestRegression_LivePathUnchangedRaw (§13.3).
	if now := nowMonotonic(); now.Sub(s.lastFgSampleAt) >= 250*time.Millisecond {
		s.lastFgSampleAt = now
		s.checkForegroundResetLocked() // may model.Write the teardown seq; never touches the live wire
	}
}

// writeModelLocked feeds a chunk into the model under s.mu. The SUBSTANTIVE panic recovery
// lives in the adapter (vtModel.Write, §4.1): on a parse panic the adapter RECREATES the
// emulator to a known-blank, fully-parseable state at the current size (the parser may be
// corrupted mid-sequence — "one chunk behind" is NOT a safe assumption — so we do not
// continue on a possibly-poisoned parser), sets a sticky `degraded` flag, and preserves the
// compact scrollback ring. The model is therefore guaranteed PARSE-CORRECT after a recovered
// panic; the running app repaints on its next frame and live clients already got the raw
// bytes verbatim. This session-level recover is a DEFENSE-IN-DEPTH BACKSTOP only: it catches
// a panic that somehow escapes the adapter (e.g. inside the drainEvictedLine eviction hook, or a re-panic) so
// it can never strand s.mu (which would deadlock every later Attach/Write/Resize/Serialize/
// pumpStep on this session forever) and never stall live output (already enqueued by
// fanOutLocked before this runs). It bumps a metric and continues; the session is NOT killed.
func (s *Session) writeModelLocked(chunk []byte) {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++ // metric; logged once at warn, not per-chunk spam
			// s.mu is still held (defer in pumpStep releases it); live output unaffected.
		}
	}()
	s.model.Write(chunk) // pure in-memory VT parse, no IO, no blocking; adapter self-recovers (§4.1)
}
```

- **(1) before (2)** guarantees live clients are served before any modeling work. A
  client's `send` is a buffered channel with a **non-blocking** `select` (drop-on-overflow,
  unchanged), so fan-out is O(clients) of channel pushes — microseconds.
- **(2)** is CPU-only VT parsing of a buffer already in memory. It does **no syscalls, no
  allocations beyond grid mutation, no channel ops**. It cannot block. Worst case it is a
  few microseconds of parsing per KB. Because it runs *after* the sends, even if it were
  slower it could not delay a byte already handed to `writePump`.
- **Panic isolation (mandatory).** `defer s.mu.Unlock()` (not an explicit `Unlock`) plus
  the `recover` in `writeModelLocked` together guarantee that a `model.Write` panic on
  malformed/adversarial bytes can **never poison `s.mu`** and never stall live fan-out.
  This hazard is tracked in §14.
- The lock is held across both so `Attach` (which reads the model under `s.mu`) never
  observes a half-applied chunk, and a chunk is never both in a live frame *and* missing
  from the model — exactly the invariant the old `ring.Write`+`fanOutLocked` pairing
  guaranteed. The race-detector test (`PumpChunkForTest`) is retained and updated.

### 8.3 Serialize under the lock

`Attach` holds `s.mu` and calls `serializer.Serialize(s.model)` (a pure read, via the §8.5
`serializeLocked` recover wrapper). Because
`pumpStep` also holds `s.mu`, the snapshot is **atomic** with respect to live output: the
new client's serialized frame reflects a consistent screen, and it is enqueued *before*
the client is registered for fan-out, so no live byte is lost or duplicated across the
attach boundary (same guarantee as today's `Attach`).

**Attach at a mid-sequence boundary — the partial-sequence re-sync (Issue: orphaned
sequence-tail).** `pumpStep` chunks are PTY reads and are **NOT** escape-sequence-aligned: the
kernel routinely splits one app write across two reads (common under flood), so a chunk can end
mid-sequence — e.g. `…ESC[` with the `31mRED` tail arriving in the next chunk. The model buffers
that partial internally (the `Write` contract), and `Serialize` reflects only **committed
ground-state** screen state, so the redraw does **NOT** contain the in-flight prefix. Without a
fix, a client that attaches in that window receives the clean redraw, joins fan-out, and then
gets a first live chunk carrying only the **tail** (`31mRED`) — which its fresh `xterm`, in ground
state after the redraw, renders as literal garbage at the prompt: exactly the orphaned/garbled-
sequence class (§1.2#4, G2) reborn on the freshly-attached client. Every other boundary is
handled; this one must be too. **Mechanism (chosen): re-sync the new client to the live boundary
by appending the model's buffered partial bytes to the redraw.** Under the same `s.mu` hold,
`Attach` emits `Serialize(model)` (ending in ground state, step 17) **then**
`model.PendingInput()` — the exact raw bytes of the in-flight sequence (§4/§4.1), which every
live client already holds from the prior chunk. The new client's parser thereby enters the
**identical** mid-sequence state as the live clients, so when the live tail arrives it completes
the sequence correctly (`ESC[` + `31mRED` = `ESC[31mRED`) with **no** stream-swallow and **no**
orphaned tail. Because `PendingInput()` and `Serialize()` are both read under the one `s.mu` hold,
and the client is registered for fan-out only after both, the re-sync is atomic: the new client
sees `redraw + partial`, then the tail; the live clients saw `partial`, then the tail; both
converge. **Two boundary facts pin it:** (i) `PendingInput()` is appended **only** on the live
`Attach` path — `Snapshot()`/persistence **never** include it, because on a daemon-restart restore
the partial's tail will never arrive (a fresh shell on a fresh PTY), so baking it into the `.buf`
would re-create the §1.2#4 dangling-sequence bug; the `.buf` therefore stays self-contained and
ground-terminated. (ii) The retained partial is **bounded** at `maxPendingPartial` (4 KiB, §4.1);
a still-incomplete sequence beyond that is pathological (an app streaming a never-terminated OSC,
which strands *every* client identically — out of scope), and past the cap `PendingInput()`
returns `""`, accepting one orphaned tail that self-heals on the next repaint. The rejected
alternative — **delay registering the new client for fan-out until the parser next reaches ground
state** — cannot be done under the single `Attach` `s.mu` hold (it would require blocking on future
`pumpStep` chunks or a deferred-registration queue) and re-loses the prefix anyway unless the
partial is re-fed; the append is strictly simpler and exact. Asserted by
`TestRegression_ReattachMidSplitSequence` (§13.3) and a `model/` unit test of `trackPendingPartial`
/ `PendingInput` (§13.1).

**The Serialize hold DOES briefly exclude `pumpStep` — bounded, and we accept it.** This
is the §6 grid `Render()` (and prologue/epilogue build) running under `s.mu`, so for its
duration this **one** session's live fan-out is paused (other sessions are unaffected —
each has its own `s.mu`). We bound and minimise it:
- **Scrollback is NOT re-rendered on Serialize.** It is already stored as compact
  pre-serialized bytes in the `lineRing` (§9.2), so step 5 is a **`copy`/memcpy of up to
  `scrollbackLines` byte-slices**, not a render. The only real CPU is the **visible grid**
  `Render()` = RLE-encode of `cols × rows` cells. At 200 × 50 that is ≤ 10k cell-encodes
  ≈ **tens of µs to low-hundreds of µs**; the prologue/epilogue is a handful of fixed
  `x/ansi` writes. So one `Serialize` holds `s.mu` for **sub-millisecond**, well below
  perceptible latency, and live *bytes that were already enqueued* are never affected.
- **Frequency.** This hold occurs on `Attach` (rare, user-driven) and on the **10 s
  cadence flush** — and only when `s.dirty` (a flush of an unchanged session re-uses the
  last blob and skips Serialize entirely; see §8.4). So in steady state a quietly-busy
  session pays at most one sub-ms grid-render per 10 s. We **explicitly accept** this
  bounded stall (NG1 is updated to say so) rather than restructure the encode outside the
  lock: the grid `Render()` reads `x/vt` cell state that `pumpStep` concurrently mutates,
  so it **cannot** be moved off `s.mu` without either copying the whole grid under the
  lock (no cheaper than rendering it) or abandoning the single-lock atomicity that §8.3
  and §10(c) depend on. The single lock is the design; the bounded hold is its price.

**The model cannot lag the live stream.** Because `model.Write` runs *synchronously*
under the same `s.mu` hold as `fanOutLocked` (§8.2), after `pumpStep` returns the model
is exactly as current as the last fanned-out byte. There is **no** window in which a
client could attach and receive a serialize that is "behind" the live stream — the
stale-serialize hypothetical is impossible under this single-lock design, and §10(c) is
written to that fact. **We deliberately do NOT decouple `model.Write` to a goroutine**;
the single lock is precisely what makes this atomicity hold. (If a future perf change
ever decoupled it, this atomicity guarantee and the §10(c) reasoning would have to be
re-proven from scratch — it is not a free optimization.)

**Worst-case `s.mu` hold time (bounding client `Write`/`Resize`/`Attach` contention).**
The hold spans `fanOutLocked` (O(clients) *non-blocking* channel pushes — sub-µs) plus
`model.Write` = VT parse of the chunk + compact-serialization of any lines that chunk
evicts (§9.2). The only non-constant term is eviction encoding: a full-screen-scroll
flood evicts at most `rows` lines per screen-repaint chunk, each encoded in O(cols). At
200×50 that is ≤ 50 lines × ~200 cells ≈ 10k cell-encodes ≈ **low-tens to ~100 µs** per
screenful chunk on commodity hardware; sustained scroll amortizes to O(cols) per evicted
line. Client `Write`/`Resize`/`Attach` contend on the same `s.mu`, so their worst-case
wait is bounded at **sub-millisecond per chunk** — well below perceptible input latency —
and live *output* is already enqueued before the eviction work runs (§8.2), so eviction
encoding is never on the output path. This is the concrete cost the §10(c) flood
analysis refers to.

**`Resize` holds `s.mu` across a `pty.Setsize` syscall — bounded, and on purpose.** The
model↔PTY size invariant (§4.2) requires `pty.Setsize(cols, rows)` and
`s.model.Resize(cols, rows)` to run **under one `s.mu` hold, in the §4.2 order (`Setsize`
then `model.Resize`), with no intervening `model.Write`** — which deliberately reintroduces
a syscall under `s.mu`
that the old `Resize` avoided (it captured `ptmx` under the lock then released before the
ioctl). We **accept** that here rather than weaken the invariant: `pty.Setsize` is a single
`TIOCSWINSZ` ioctl (**single-digit µs**, same class as the `TIOCGPGRP` sample of §11.1),
plus `model.Resize` (a grid reshape, no scrollback reflow — §10(a) — so O(cols·rows), tens
of µs). `Resize` is **user-driven and rare** (a pane drag, not the per-chunk path), so this
hold occurs orders of magnitude less often than `pumpStep`. Total `Resize` hold ≈ the grid
reshape + a single-digit-µs ioctl ≈ **well under the same sub-millisecond bound** as the
`pumpStep`/`Serialize` holds above. Releasing `s.mu` before the ioctl (the old behavior) is
**rejected** because a `pumpStep` chunk parsed in the gap between `model.Resize` and
`pty.Setsize` (or vice-versa) would be parsed at a size the PTY/app disagrees with — the
exact mismatch §4.2 exists to forbid. The cheap ioctl under the lock is the price of the
invariant.

### 8.4 `Snapshot()` lock ordering (flushMu → s.mu) and model teardown

`Snapshot()` changes from lock-free (it used to read `ring.Snapshot()` under `ring.mu`
only) to **acquiring `s.mu`** and running a `Serialize`. Its callers
(`flushSessionOnce`, `persistOnDetach`, `Suspend`, `Shutdown`) call it while holding
`flushMu`. This introduces a **`flushMu → s.mu`** acquisition order, which we make a
documented invariant and prove acyclic:

- **The only orders that exist are `sessionMu → s.mu`, `flushMu → s.mu`, and (within
  `pumpStep`/`Attach`/`Resize`/`Snapshot`) a lone `s.mu`.** `s.mu` is always the
  **innermost** lock; nothing acquires `flushMu` or `sessionMu` *while holding* `s.mu`.
  In particular `Snapshot()` takes `s.mu`, calls `Serialize` (pure model read, takes no
  further lock), and releases `s.mu` before returning to its `flushMu`-holding caller. So
  `flushMu` and `s.mu` are never acquired in the opposite order and `pumpStep`
  (`s.mu` alone) can never be blocked waiting on `flushMu`. The graph is a DAG → no
  deadlock. `Snapshot()`'s doc-comment is updated to state "acquires `s.mu`; callers must
  hold `flushMu` and MUST NOT already hold `s.mu`."
- **Cadence-flush skip — single combined dirty-gated serialize under ONE `s.mu` hold,
  returning a `changed` write-skip signal.** The old two-method pattern (`TakeDirty()`
  taking `s.mu` once, then `Snapshot()` taking `s.mu` again) is **removed**: a chunk arriving
  in the gap between `TakeDirty()` clearing the bit and `Snapshot()` running would have its
  dirty bit already consumed, so the *next* flush would skip it and the latest state would
  never persist. Instead `Snapshot()` itself performs the **check-and-clear + serialize
  atomically within a single `s.mu` acquisition**, and — critically — **returns whether the
  state actually changed since the last flush**, so the persistence layer can skip the *disk
  write*, not merely the serialize:

  ```go
  // Snapshot returns (header+blob, changed) for the persistence paths. ONE s.mu hold does
  // the foreground-reset sample (§11.1), the dirty check, the serialize, and the clear — so
  // no pumpStep chunk can slip between "dirty consumed" and "state captured".
  //   changed == true  → state moved since the previous flush; callers SHOULD WriteBuf+saveMeta.
  //   changed == false → identical to the last persisted blob; the cadence flush MUST skip
  //                       the WriteBuf AND the saveMeta (no disk I/O for an idle session).
  // The dirty bit is consumed in THIS same hold, so `changed` is exactly the bit value.
  func (s *Session) Snapshot() (blob []byte, changed bool) {
      // PLACEHOLDER FAST-PATH (Issue): a placeholder builds NO vtModel and NO serializer
      // (§4.2), so it MUST return its stored raw blob BEFORE any model/serializer access —
      // otherwise the s.serializeLocked() line below nil-panics for every
      // placeholder Snapshot. s.model == nil is the discriminator (only placeholders have a
      // nil model). s.rawBlob is the placeholder's persisted serialized blob (the dedicated
      // field, distinct from the live-session lastBlob cache — see the field note below); it
      // never moves (a placeholder parses no bytes), so changed == false and a cadence flush
      // writes nothing (§8.4 last bullet). No s.mu, no Serialize — a pointer return.
      if s.model == nil {
          return s.rawBlob, false
      }
      s.mu.Lock()
      defer s.mu.Unlock()
      s.checkForegroundResetLocked()      // may model.Write → sets s.dirty (§11.1)
      if !s.dirty && s.lastBlob != nil {  // unchanged since last flush → reuse, skip Serialize AND signal no-write
          return s.lastBlob, false
      }
      blob = s.header() + s.serializeLocked() // §8.5 recover wrapper (not serializer.Serialize
                                          // directly); header-only on a recovered Serialize panic — the §8.3 grid-render hold
      s.lastBlob = blob
      s.dirty = false                     // cleared in the SAME hold that produced blob
      return blob, true
  }
  ```

  So the §8.3 grid-render hold is paid only by genuinely-dirty sessions (and `Attach`), the
  dirty flag is cleared in the **same** `s.mu` hold that produced the blob (closing the gap),
  and `pumpStep`'s `s.dirty = true` (§8.2) can only land *before* or *after* this critical
  section, never inside it. `flushSessionOnce` calls this `Snapshot()` directly; there is no
  separate `TakeDirty()`.
- **`flushSessionOnce` skips ALL disk I/O for an unchanged session — closing the I/O
  regression.** Today `flushSessionOnce` uses `TakeDirty()` to skip `WriteBuf`+`saveMeta`
  entirely for an idle session. Under the new design it gates on the `changed` return:

  ```go
  blob, changed := s.Snapshot()
  if !changed {
      return // idle session: NO WriteBuf, NO saveMeta this tick — exactly today's dirty-gated behavior
  }
  persistence.WriteBuf(s.id, blob)
  s.saveMeta(...)
  ```

  Without this, an idle session would `WriteBuf` the same ~full blob plus `saveMeta` every
  10 s — for 100 sessions, a large continuous I/O regression versus today. The `changed`
  bit makes a quiet session cost **zero** cadence disk writes, matching today's dirty gate.
  **The terminal-event callers (`persistOnDetach`, `Suspend`, `Shutdown`) deliberately
  IGNORE `changed` and always persist** — a detach/suspend/shutdown must flush the latest
  state to disk regardless of the cadence dirty bit (e.g. a session that went idle long ago
  but was never flushed since restart still needs its blob on disk at suspend). Only the
  10 s cadence `flushSessionOnce` honors the skip. `TestRegression_IdleSessionNoCadenceWrite`
  (§13.3) asserts an unchanged session performs **no** `WriteBuf`/`saveMeta` on a cadence tick.
- **The four production `Snapshot()` call sites + the `TakeDirty()` removal, enumerated to the
  §9.1 standard (Issue: the `(blob, changed)` signature change is a compile break at every caller
  and must not be rediscovered piecemeal).** `Snapshot()` returns `(blob []byte, changed bool)`
  instead of `[]byte`, so each call site below is updated as stated (verified against the tree at
  spec time); each line says whether it **consumes** or `_`-**ignores** `changed`:
  - **`flushSessionOnce` (`terminal.go:1185`) — CONSUMES `changed`.** `snap := s.Snapshot()`
    becomes `blob, changed := s.Snapshot()` and the body gains the `if !changed { return }` skip
    (above) so an idle cadence tick writes nothing. **The `TakeDirty()` gate at `terminal.go:1170`
    (`if !s.TakeDirty() { return }`) is DELETED**, and `Session.TakeDirty()` is removed from
    `session.go`: its check-and-clear moves *into* `Snapshot()`'s single-`s.mu`-hold `changed`
    computation (§8.4 above), closing the chunk-in-the-gap race the old two-method pattern had.
    This is the **sole** caller that honors the skip.
  - **`persistOnDetach` (`terminal.go:815`) — `_`-IGNORES `changed`.** `snap := s.Snapshot()`
    becomes `blob, _ := s.Snapshot()`; a detach always persists the latest state regardless of the
    dirty bit.
  - **`suspend` (`terminal.go:702`) — `_`-IGNORES `changed` (and the force path is REPLACED).**
    The non-force/idle path becomes `blob, _ := s.Snapshot()` and always persists. The **force**
    path no longer calls `s.Snapshot()` at `:702` at all: it is replaced by
    `forceSuspendSnapshotLocked` (§11.2), which fuses the notice + teardown + serialize in one
    `s.mu` hold and returns the blob, and `BeginForceSuspend` persists *that* blob without
    re-`Snapshot()`ing (`TestRegression_ForceSuspendInterleaveRace`). The old raw
    `append(scrollback, "\r\n[crowbar] … suspended …\r\n")` byte mutation at `:704` is **removed**
    (it would corrupt the blob — the notice now enters via `InjectLocal`, §12).
  - **`Shutdown` (`terminal.go:1446`) — `_`-IGNORES `changed`.** `snap := s.Snapshot()` becomes
    `blob, _ := s.Snapshot()`; shutdown always flushes the latest state to disk.
  So exactly one caller (`flushSessionOnce`) honors the write-skip; the three terminal-event
  callers always `WriteBuf`. Any test caller of `Snapshot()` adopts the two-value form the same
  way (`blob, _ :=`). The placeholder fast-path (`s.model == nil` ⇒ `(s.rawBlob, false)`) means a
  cadence flush of a placeholder also writes nothing, consistent with the skip.
- **The cached blob is a real `Session` field, counted in the memory ceiling — and is
  DISTINCT from the placeholder blob field (Issue).** Two separate `[]byte` fields are added
  to the `Session` struct (§3.3), one per session kind, never aliased:
  - **`s.lastBlob []byte`** — the **live-session** cadence-flush cache (this section): the
    last `header + Serialize(model)` output, re-serialized on demand, reclaimable under
    pressure (§9.4), and gated by `s.dirty bool`. It is meaningful only when `s.model != nil`.
  - **`s.rawBlob []byte`** — the **placeholder's** persisted serialized blob (§4.2/§12): the
    verbatim `.buf` bytes a suspended, model-less placeholder holds. It is immutable for the
    placeholder's lifetime (a placeholder parses no bytes) and is what the placeholder
    `Snapshot()` fast-path returns.
  Keeping them separate makes `ModelBytes()` (§9.4) unambiguous per kind: a live session
  counts `m.ModelBytes() + len(s.lastBlob)` (and `s.rawBlob == nil`), a placeholder counts
  `len(s.rawBlob)` (and `s.lastBlob == nil`). Both are accounted so the resident-memory
  ceiling is not undercounted.
- **Every state mutation that the cache must not outlive sets `s.dirty` (and `Resize` also
  nils `s.lastBlob`).** The complete set of `s.dirty = true` sites is: `pumpStep` (§8.2),
  `checkForegroundResetLocked` (§11.1), `InjectLocal` (§12), and **`Session.Resize`**
  (§4.2/§10(a)). `Resize` is the one mutation that produces **no blob bytes of its own** yet
  changes the header's `cols×rows`, so it additionally clears `s.lastBlob = nil` in the same
  hold — otherwise the `!s.dirty && s.lastBlob != nil` fast-path could (if a stray reorder
  ever left dirty unset) hand back a stale-size blob. Setting dirty AND nil-ing the cache on
  resize makes the next `Snapshot()` unconditionally re-serialize at the new size. This is
  why a resize-only, output-free session still persists a fresh, correct-size `CRWB1` header
  (`TestRegression_ResizeOnlyPersistsNewSize`, §13.3).
- **Placeholder `Snapshot()` takes no model lock and reports `changed == false`.** A
  suspended placeholder holds only `s.rawBlob` (§4.2, the named field above) and has
  `s.model == nil`; the canonical `Snapshot()` fast-path above returns `(s.rawBlob, false)`
  verbatim with no `Serialize` and no `s.mu` model access — a pointer return, outside this
  ordering concern. `changed == false` because a placeholder's persisted blob never moves
  (it parses no bytes), so a cadence `flushSessionOnce` of a placeholder correctly performs
  no disk write.
- **`TerminalModel.Close()` is wired into session teardown.** `shutdown()` (and the reap
  path) call `s.model.Close()` under `s.mu` after the pump goroutine has exited, so no
  concurrent `model.Write` can race it. For the `x/vt` backend `Close()` only resets the
  shadow and drops the emulator reference (x/vt holds no OS resource), so it is effectively
  a GC hint and is idempotent; we still call it explicitly so a future backend with real
  resources (file handles, cgo) has a defined teardown hook rather than relying on GC.

### 8.5 Panic isolation across ALL model-access paths (not just `Write`)

The no-kill guarantee — "a model panic can never strand `s.mu` and never kill the session; it
bumps a metric and continues" — must hold on **every** path that touches the model, not only
`pumpStep`'s `writeModelLocked`. `Serialize`/`Resize`'s eviction hook (`drainEvictedLine`, fired
inside `emu.Resize`)/`OnForegroundReset`'s
teardown each touch the (untagged, ~2-week-old) emulator and can panic, and a downcast
mismatch in `Serialize` panics too (§4). **An escaping panic on any of these unwinds to
`pump()`'s `safego.Recover → defer s.shutdown()` (for the pump goroutine) or to the readPump/WS
goroutine (for client `Resize`), KILLING or crashing the session — contradicting the guarantee.**
`defer s.mu.Unlock()` alone does **not** recover; it only unlocks. So the recover boundary is made
to **match the claim** with two small session helpers that every model access routes through:

```go
// serializeLocked runs serializer.Serialize under a recover so a Serialize/downcast panic
// (§4) can never escape. Caller holds s.mu. On panic it bumps modelPanics and returns nil; the
// caller treats nil as "no redraw this time" (Attach still registers the client and serves
// live; a flush/suspend falls back to lastBlob or persists header + empty body). The adapter's
// own degraded flag (§4.1) is already sticky. NEVER re-panics. This is the SOLE Serialize
// call path — Attach (Appendix A), Snapshot (§8.4), and forceSuspendSnapshotLocked (§11.2) all
// call serializeLocked, never serializer.Serialize directly.
func (s *Session) serializeLocked() (redraw []byte) {
    defer func() {
        if r := recover(); r != nil { s.modelPanics++; redraw = nil }
    }()
    return s.serializer.Serialize(s.model)
}

// mutateModelLocked runs a VOID model mutation (Resize's model.Resize+drain, OnForegroundReset's
// teardown) under the same recover backstop. Caller holds s.mu. On panic it bumps modelPanics and
// returns; the mutation is partially applied but the session keeps serving (the adapter self-heals
// via recreateEmu on the Write path; a Resize/teardown panic leaves a consistent-enough model that
// the next app frame repaints). NEVER re-panics.
func (s *Session) mutateModelLocked(fn func()) {
    defer func() {
        if r := recover(); r != nil { s.modelPanics++ }
    }()
    fn()
}
```

- **`writeModelLocked` (§8.2) is the `Write`-path instance** of this same pattern (it already
  recovers, bumps `modelPanics`, and relies on `pumpStep`'s `defer s.mu.Unlock()` for the lock).
  It is unchanged; §8.5 generalises the same backstop to the other model-access paths.
- **`Session.Resize`** routes its model mutation through `mutateModelLocked(func(){
  s.model.Resize(cols, rows) })` — so a `drainEvictedLine` (eviction-hook) panic fired during the
  resize's `emu.Resize` is caught and the readPump/WS goroutine is never crashed. (`pty.Setsize`
  is a syscall, outside the model recover; its error is handled as today.)
- **`checkForegroundResetLocked`** routes its `s.model.OnForegroundReset()` call through
  `mutateModelLocked`, so the app-death-edge teardown (which feeds synthetic bytes through the
  model and runs in `pumpStep` step (3), in `Snapshot()`, and in `runMaintenanceOnce` — all
  **outside** `writeModelLocked`'s recover) cannot escape on any of its sampling sites.
- **`Attach`, `Snapshot()`, and `forceSuspendSnapshotLocked`** obtain the redraw from
  `serializeLocked()` (never `serializer.Serialize` directly), so a Serialize/downcast panic on
  any serialize path bumps the metric and leaves the session serving — Attach registers the client
  with no redraw frame, a flush reuses `lastBlob`, a suspend persists a header-only blob.
- **`injectLocalLocked` (§12, the notice-injection path) is in this enumerated recover set —
  its `model.Write`s route through `writeModelLocked`.** `injectLocalLocked` performs up to two
  `model.Write`s (the defensive drive-to-primary alt-exit sequence, then the daemon-authored
  notice bytes); **both go through `writeModelLocked`** (the §8.2 `Write`-path recover), so a
  parse panic on a synthetic notice bumps `modelPanics` and returns instead of escaping. This is
  load-bearing because `injectLocalLocked` is reachable from **two callers, neither of which has
  its own model recover on the unwinding path**: (1) `InjectLocal` on the **restore path**, called
  from the engine-level `e.spawn` **before `reg.Add`** (§9.1/§12) — an escaping emulator panic
  there would unwind into the engine spawn/restore goroutine, which has **no** model recover, and
  crash the restore; and (2) `forceSuspendSnapshotLocked` (§11.2), whose teardown serialize is
  already under `serializeLocked` but whose **fused notice `model.Write` must be recovered too**.
  Routing both notice writes through `writeModelLocked` makes "a notice-injection parse panic can
  never escape to the spawn/restore or force-suspend goroutine" a guarantee, not a hope. (The
  primary-forcing guard's alt-exit write is itself a `model.Write`, so it is covered by the same
  wrapper.)
- **Lock safety is unchanged:** all these helpers run **inside** the caller's existing `s.mu` hold
  whose `defer s.mu.Unlock()` still releases the lock after the (now-recovered) panic, so `s.mu`
  can never be stranded. The §14 risk row's "a panic that somehow escapes the adapter e.g. inside
  the `drainEvictedLine` eviction hook" is now genuinely caught on the `Write` **and** `Resize` paths, and the §4
  Serialize-downcast panic is caught by `serializeLocked`, not by a bare unlock.

`TestRegression_ModelPanicOnResizeAndAttachStillServes` (§13.3) injects a panic on the **Resize**
drain path and on the **Attach serialize** path (via the `newEmulator` seam, §4.1) and asserts
each bumps `modelPanics`, does **not** kill the session, and the session still serves the next
`Attach`/`Write`.

---

## 9. Scrollback design

### 9.1 Depth tied to the setting — and the *actual* wire change

The frontend exposes `terminalScrollback` (default **10000** lines,
`web/src/features/settings/config/default-settings.ts:40`) and constructs `xterm.js`
with `scrollback: terminalScrollback` (`terminal.tsx:261`). **Today this value lives
ONLY on the client — the daemon never receives it.** The `Create` RPC carries a
`*domain.TerminalProfile`, which has **no scrollback field**, and nothing transmits the
setting. This is therefore **new plumbing we must build**, not an existing path:

1. **Transient create DTO, NOT the persisted profile row.** `scrollbackLines`,
   `initialCols`, and `initialRows` are **per-session, ephemeral** values — they describe
   *this* attach's window, not a durable user preference. `domain.TerminalProfile` is a
   **GORM-persisted table** (it defines `TableName()`); adding columns to it would create
   a durable schema migration *and* conflate ephemeral window geometry with a saved
   profile (and would re-open the "no migration" stance). So we **do not** touch the
   `TerminalProfile` row. Instead, `Engine.Create` gains a **transient request DTO**
   carried alongside the profile:
   ```go
   // CreateRequest is a per-call, NON-persisted DTO. It never reaches GORM.
   type CreateRequest struct {
       Profile         *domain.TerminalProfile // existing, persisted (unchanged schema)
       InitialCols     int                     // this attach's measured xterm width  (§4.2)
       InitialRows     int                     // this attach's measured xterm height (§4.2)
       ScrollbackLines int                     // resolved terminalScrollback for this session
   }
   ```
2. **FE send.** The create payload the FE already builds now also carries
   `scrollbackLines` (the same resolved `terminalScrollback` it passes to `xterm.js`) and
   the measured `initialCols`/`initialRows` (§4.2), as fields of the create request — not
   on the persisted profile.
3. **Engine `Create` carries the DTO; the dims must reach it from the wire.** `Engine.Create`
   gains the trailing `CreateRequest` and reads `req.InitialCols` / `req.InitialRows` /
   `req.ScrollbackLines` to pass to `session.New`. This signature change ripples to **every
   `Create` declaration, every `Create` call site, the RPC request decode that fills the DTO,
   AND the parallel usecase `CreateSession` surface** — all of which MUST be updated together or
   the build breaks. The earlier draft enumerated only the inner `engine.Create` line and
   omitted the **actual conduit** by which FE-measured cols/rows/scrollback reach the engine; the
   **complete, verified surface** (against the tree at spec time) is:

   - **(3a) The two create-RPC request DTOs + decode — where cols/rows/scrollback live in the
     JSON (this was missing).** The dims arrive in the POST body and must be decoded and threaded
     into the engine call:
     - `internal/api/v0/endpoints/terminal/handlers/sessions.go:42` — the workspace-scoped create
       handler's request struct grows from `struct{ ProfileID string \`json:"profileId"\` }` to
       additionally carry **`Cols int \`json:"cols"\``, `Rows int \`json:"rows"\``, and
       `ScrollbackLines int \`json:"scrollbackLines"\``**. The existing EOF-tolerant
       `ShouldBindJSON` decode is unchanged in shape; the handler then builds
       `CreateRequest{Profile: prof, InitialCols: body.Cols, InitialRows: body.Rows,
       ScrollbackLines: body.ScrollbackLines}` and passes it to `eng.Create` (the call at
       `sessions.go:52`). Omitted/zero JSON fields default per (3e).
     - `internal/api/v0/endpoints/home/handlers/terminal.go:53` — the home/quick-terminal create.
       Today it passes a `nil` profile and **no body/dims**; it now builds
       `CreateRequest{Profile: nil, InitialCols: 80, InitialRows: 24, ScrollbackLines: 0}`, i.e.
       **defaults to 80×24** and lets `resolveScrollback(0)` → 10000 (no fitted `xterm` to
       measure). (If the home create later grows a measured-dims body, it threads them the same
       way as 3a's workspace handler; until then the default is explicit.)
   - **(3b) The THREE interfaces that declare `Create`, plus the concrete engine.** `Create` is
     declared on three independent interfaces, all satisfied by the same `*terminalEngine`; **all
     three** adopt the `CreateRequest` trailing arg or they fail to compile against the new
     concrete signature:
     - `internal/api/v0/endpoints/terminal/handlers/handlers.go:18` — the terminal-handlers
       `TerminalEngine` interface (the RPC path's engine surface, called at `sessions.go:52`).
     - `internal/api/v0/endpoints/home/handlers/handlers.go:94` — the home-handlers
       `TerminalEngine` interface (called at `home/handlers/terminal.go:53`).
     - `internal/app/usecases/terminal/terminal.go` — the usecase-layer `Engine` interface (the
       `Create` method the usecase depends on, called at `terminal.go:130`).
     - **Concrete impl:** `internal/engine/terminal/terminal.go:286` (`func (e *terminalEngine)
       Create`) adopts `(ctx, workspaceID, workspaceDir string, req CreateRequest)` and threads
       `req` into `e.spawn` via the `engineBirth` DTO (step 4 / §9.1 final-construction-API).
   - **(3c) The parallel usecase `CreateSession` surface ALSO gains the dims (this was missing).**
     `terminalUsecase.CreateSession` is a public method on the `Usecase` interface that exists
     alongside the handler path (it is the create surface the usecase exposes and that the
     restart/round-trip and unit tests drive). Its signature changes from
     `CreateSession(ctx, wsID string, prof *domain.TerminalProfile)` to
     `CreateSession(ctx, wsID string, prof *domain.TerminalProfile, cols, rows, scrollbackLines int)`,
     in **both** the interface declaration (`terminal.go:51`) and the implementation
     (`terminal.go:121`), and the impl builds the `CreateRequest` it passes to `u.engine.Create`
     at `terminal.go:130`. Every `CreateSession` caller updates: the test callers
     `internal/app/usecases/terminal/restart_roundtrip_test.go:199` and `:387`, and
     `internal/app/usecases/terminal/terminal_test.go:50/66/77` (which today pass `nil`) now pass
     `nil, 0, 0, 0` (defaults → 80×24, `resolveScrollback(0)`). (The workspace RPC handler does
     **not** route through `CreateSession` — it calls `eng.Create` directly per 3a/3b — so
     `CreateSession` carrying the dims keeps the two create surfaces consistent rather than adding
     a second wiring path.)
   - **(3d) Mocks + test doubles (all adopt the new `Create`/`CreateSession` shape to compile).**
     - `internal/app/usecases/mocks/mocks.go:788` — the `Engine.Create` mock.
     - `internal/api/v0/endpoints/terminal/handlers/error_test.go:21` — `nilEngine.Create`.
     - `internal/api/v0/endpoints/terminal/handlers/handlers_test.go:31` — `stubEngine.Create`
       (satisfies the terminal-handlers `TerminalEngine`); the create-handler tests at
       `handlers_test.go:222` add cols/rows/scrollback to the POST body they send.
     - `internal/api/v0/endpoints/terminal/routes_test.go:27` — `stubEngine.Create`.
     - `internal/api/v0/container_terminals_test.go:63` — the
       `engContainer.Terminal.Create(ctx, "w1", worktree, nil)` call site → pass a `CreateRequest`.
     - `internal/engine/terminal/terminal_test.go` — the engine's own `eng.Create(ctx, …, nil)`
       call sites (multiple) → pass a `CreateRequest`.
     - Any home-handler test stub satisfying the home `TerminalEngine` (mirrors the terminal stub).
   - **(3e) Default behavior when the RPC omits the fields.** A zero/absent `cols`/`rows` defaults
     to **80 × 24** and a zero/absent `scrollbackLines` resolves via `resolveScrollback(0)` → 10000
     (§9.1 step 5), applied **identically** to PTY and model (§4.2). This mirrors the
     not-yet-measurable FE case and the headless/home create, so a client that sends no dims is
     never a corruption case — it just births at the historical 80×24 default.

   This is the full compile-breakage surface, now including the RPC decode, all three `Create`
   interfaces, and the usecase `CreateSession`; it is enumerated here so it is not discovered
   piecemeal. **Restore (`NewRestored`) and placeholder load take NONE of these from a DTO** —
   their `scrollbackLines` and size come from the §12 `.buf` header (§4.2, §12), the single
   authoritative restore source.
4. **`spawn()` signature (shared by create and restore).** `spawn()` is the one helper that
   starts the PTY, sizes it, builds the model, and starts the pump for **both** paths, so it
   must carry **either create-dims OR a restore blob** (never both). Its revised signature:
   ```go
   // spawnParams carries exactly one of the two birth modes (§4.2):
   //   create : Blob == nil → size+scrollback come from Cols/Rows/ScrollbackLines.
   //   restore: Blob != nil → size+scrollback come from the CRWB1 header parsed HERE
   //            (the one authoritative header parse, §12); Cols/Rows/ScrollbackLines ignored.
   type spawnParams struct {
       Cols, Rows      int    // create-dims (create path only)
       ScrollbackLines int    // create path only
       Blob            []byte // restore path only (raw .buf bytes incl. CRWB1 header)
   }
   func (s *Session) spawn(p spawnParams) error
   ```
   `spawn()` calls `pty.Start`, then `pty.Setsize` with the create-dims (create) or the
   header dims (restore) **before the first `Read`** (§4.2), builds the model at that size +
   `scrollbackLines`, and for the restore path `model.Write(redraw)` after splitting the
   header. The **session-level** `spawn()` itself stays notice-free and minimal (no notice
   parameter); the restore notice is injected by the **engine-level** `spawn` **between
   `session.NewRestored` and `e.reg.Add`** — i.e. **before the session is registered and
   therefore before any concurrent `Attach` can observe it** (§12 race fix), not "after
   `spawn()` returns." `InjectLocal` is model-only, so the notice never touches the persisted
   `.buf`.

   **Final construction API (reconciling §3.3/§4.2 constructors, the session-level
   `spawn`, and the existing engine-level `spawn` at `terminal.go:261` — Issue).** The two
   vocabularies are unified explicitly; the implementer does not have to infer the wiring:
   - **Three session constructors** — the only session-birth surface the engine calls. The
     existing `New`/`NewRestored`/`NewPlaceholder` (`session.go:79/104/140`) keep their names
     but gain the model-era parameters:
     ```go
     // CREATE: thin — allocates the Session, then s.spawn(create-dims). (replaces the old
     // New that did pty.Start inline and held a ring.)
     func New(id, shell, cwd, profileID string, env []string, cols, rows, scrollbackLines int) (*Session, error)

     // RESTORE: thin — allocates the Session, then s.spawn(restore-blob); spawn parses the
     // CRWB1 header for size+scrollback and model.Write(redraw). Takes the RAW blob, NOT dims
     // (§4.2/§12: the header is the sole authoritative restore size). (replaces the old
     // NewRestored(...scrollback []byte) that pre-loaded a ring.)
     func NewRestored(id, shell, cwd, profileID string, env []string, rawBlob []byte) (*Session, error)

     // PLACEHOLDER: does NOT call spawn — no PTY, no vtModel. Stores rawBlob in s.rawBlob
     // (§4.2/§8.4) and may read the CRWB1 header for size metadata. (replaces the old
     // NewPlaceholder that lazily sized a ring.)
     func NewPlaceholder(id, shell, cwd, profileID string, rawBlob []byte) *Session
     ```
   - **`New` and `NewRestored` are thin wrappers over the single session-level `s.spawn`**:
     each builds the bare `Session` (the shared `newSession` allocator), then calls
     `s.spawn(spawnParams{...})` (create-dims for `New`, `Blob: rawBlob` for `NewRestored`)
     and returns. `spawn` is the SOLE place `pty.Start` + `pty.Setsize` + model build +
     `model.Write(redraw)` (restore) + `go s.pump()` happen, for BOTH paths — so the
     size-before-first-Read invariant (§4.2) and the pump-after-model-write ordering live in
     one helper. `NewPlaceholder` bypasses `spawn` entirely (it has no PTY to start).
   - **The engine-level `spawn` (`terminal.go:261`) keeps its name and role** (register +
     `reapOnDone`) but its trailing `scrollback []byte` param becomes an `engineBirth` DTO
     carrying exactly one birth mode (mirroring `spawnParams` at the engine boundary), and it
     threads the new create-dims through to `session.New`:
     ```go
     // engineBirth carries exactly one of the two birth modes to e.spawn (Blob==nil ⇒ create).
     type engineBirth struct {
         Cols, Rows, ScrollbackLines int    // create path
         Blob                        []byte // restore path (raw .buf incl. CRWB1 header)
         Notice                      []byte // restore path only: daemon-authored on-screen notice
                                            // (e.g. "[crowbar] restored …"), injected into the model
                                            // BEFORE reg.Add exposes the session for attach (§12 race fix)
     }
     func (e *terminalEngine) spawn(id, workspaceID, shell, cwd, profileID string, b engineBirth) (*session.Session, error) {
         var s *session.Session; var err error
         if b.Blob != nil {
             s, err = session.NewRestored(id, shell, cwd, profileID, ptyEnv(), b.Blob)   // restore
         } else {
             s, err = session.New(id, shell, cwd, profileID, ptyEnv(), b.Cols, b.Rows, b.ScrollbackLines) // create
         }
         if err != nil { return nil, err }
         if len(b.Notice) > 0 {
             s.InjectLocal(b.Notice) // §12: model-only notice, injected BEFORE reg.Add so the very
                                     // first concurrent Attach cannot observe the registered session
                                     // without the notice already on the model (Issue: reg.Add makes
                                     // the session attachable, so injecting AFTER spawn returns races).
         }
         e.reg.Add(id, workspaceID, s); go e.reapOnDone(id, workspaceID, s); return s, nil
     }
     ```
     Its two call sites change accordingly: `Engine.Create` (`terminal.go:295`) passes
     `engineBirth{Cols: req.InitialCols, Rows: req.InitialRows, ScrollbackLines:
     resolveScrollback(req.ScrollbackLines)}` (from the §9.1 `CreateRequest`; no `Notice`); the
     restore call site (`terminal.go:566`) passes `engineBirth{Blob: rawBlob, Notice:
     resolveRestoreNotice(...)}` (the raw `.buf` bytes, no longer the pre-split `scrollback`, plus
     the `[crowbar] restored …` notice the old `restoreCWD` used to append to bytes — §12). The
     old `scrollback []byte` "non-nil ⇒ restore" branch is exactly preserved in shape
     (`Blob != nil ⇒ restore`), so the engine control flow is otherwise unchanged. Because the
     notice is injected **inside** `e.spawn` before `reg.Add`, the §12 "notice lands before any
     attach can observe the session" invariant is **actually achievable** — it does not depend on
     the unlucky timing of a separate post-`spawn` `InjectLocal`.
   So: `engineBirth` (engine boundary) → `New`/`NewRestored` (thin) → `spawnParams` (session
   boundary) → `s.spawn` (the one shared birth helper). `NewPlaceholder` stands apart with no
   PTY and no model.
5. **Fallback.** When a create field is zero/unset (not-yet-measurable FE, headless/test
   create) default in exactly one place each:
   `resolveScrollback(n int) int { if n <= 0 { return 10000 }; return n }`, and size
   defaults to 80 × 24 (§4.2). For restore, a header that is malformed or carries a
   zero scrollback is likewise resolved through `resolveScrollback`.

`newVTModel(cols, rows, scrollbackLines)` uses this value as the depth of the
**compact-line ring** (the authoritative scrollback — §9.2), **not** as x/vt's cell
scrollback. Changing the setting affects new sessions only; existing sessions keep their
depth until re-created (matches `xterm.js`, which also only applies depth at construction).

### 9.2 Stored as compact serialized lines — and x/vt retains none (`WithScrollback(0)`)

A naïve emulator stores scrollback as **cell grids** (each cell = rune + fg + bg + attrs,
~8–16 bytes). At 10000 lines × 200 cols × ~12 bytes that is **~24 MB per session** —
catastrophic at the 100-session ceiling. We avoid it, but **only if x/vt is not ALSO
asked to keep a deep cell scrollback.** The trap called out in review: constructing
`vt.WithScrollback(10000)` makes x/vt keep its OWN ~24 MB cell scrollback, so a
compact-line ring layered on top would be a *second* copy, not a replacement. We build x/vt
with `WithScrollback(0)` — zero retained cell scrollback — and capture every evicted line
through the pre-scroll/eviction hook instead (the SOLE drain mechanism; the `ScrollbackLen()`-diff
alternative is deleted, see "Drain mechanism" below). We resolve both halves explicitly:

- **The authoritative scrollback is the adapter's own bounded compact-line ring**
  (`lineRing`, cap = `scrollbackLines`). Each ring entry is a `(bytes []byte, wrapped bool)`
  pair: the compact clean-ANSI encoding of the evicted physical row (§9.2.1) **plus** the
  row's soft-wrap flag (true when the logical line continues on the next physical row). The
  `wrapped` flag is what step 5 consults to decide whether to emit a `\r\n` separator (Issue:
  omitting it between a wrapped row and its continuation is what preserves wrap state and
  physical line-count against the `SerializeAddon` oracle). When a row scrolls off the top of
  the live grid the model encodes it **once** (§9.2.1) — the exact bytes step 5 emits — and
  appends the pair. A typical row is a few dozen bytes, so the footprint is **comparable to
  the old raw ring**, not the cell-grid blowup. Rows are immutable (no reflow — §10(a)), so
  encode-once-on-eviction is correct and cheap.
- **Wrap-flag source — x/vt's per-row wrap signal is the SOLE source, and exposing it is a
  HARD P0 ADOPTION GATE (blocker resolution).** The `wrapped` flag is read **directly from x/vt's
  own per-row wrap signal**, split by row kind because the two kinds have different ceilings:
  - **(i-a) EVICTED scrollback rows — the pre-scroll / line-eviction hook, REQUIRED.** The hook
    hands `drainEvictedLine` the leaving row's `uv.Cell`s *and* its `wrapped` bit **at the instant
    it scrolls off the primary grid, before x/vt drops it** (the `WithScrollback(0)` mechanism,
    "Drain mechanism" below). This is the **only** evicted-row source, because it fires per line
    with no per-`Write` ceiling — a single large `Write` (restore replay, flood, `CSI Ps S`)
    still delivers every evicted line's cells+wrap in order. The deleted `ScrollbackLen()`-diff +
    `ScrollbackRowWrapped(y)` path is rejected precisely because a retained-window getter could
    read at most ~256 of the lines a big `Write` evicts (same ceiling as the line-capture hole).
  - **(i-b) VISIBLE grid rows — `RowWrapped(y)`, read at paint time.** The live grid is a bounded
    `rows×cols` buffer with **no eviction ceiling**, so each visible row's wrap bit is read on
    demand from x/vt at serialize/paint time (or, equivalently, `Render()` emits a wrapped grid
    row continuously with no `CR`/`LF`). These reads are always consistent with the cells
    `encodeRow` serializes for the same row.

  Adoption requires **both** (i-a) and (i-b). There is no retained-window scrollback getter in the
  design (it would reintroduce the capture ceiling); evicted-row wrap flags come **only** from the
  hook.

  **Why there is NO adapter-maintained side array (the unsound fallback is deleted).** A
  per-physical-row side array would have to be maintained *inside* `Write` by observing a
  per-print, sub-chunk event — "printing advanced the cursor off row `y`'s last column with
  DECAWM ON and no intervening `CR`/`LF`/cursor-move" — which depends on cursor position,
  glyph width (wide CJK), SGR/charset byte skipping, and scroll-region clamping. But the
  adapter **does not parse `p`**: it calls `emu.Write(p)` (x/vt owns the parse) and reads
  getters only at **chunk boundaries**. A single 64 KB PTY read (the `cat`/flood case — exactly
  where wrap matters) prints and scrolls **many full screens within one `Write`**, so
  chunk-boundary `CursorPosition()`/`ScrollbackLen()` deltas **cannot** reconstruct which rows
  soft-wrapped before they evicted. Maintaining such an array correctly would require a second,
  full semantic VT cursor/width/scroll tracker running in lock-step with x/vt — i.e.
  re-implementing the emulator we adopted x/vt to avoid. That is rejected as unsound. Wrap
  fidelity (G3 / the §13.2 `softwrap` oracle) therefore has **exactly one** correct source:
  x/vt's own per-row signal.

  **The gate.** If the pinned x/vt commit exposes **no eviction hook (i-a)** (and no thin
  wrapper around its scroll path can synthesise one), then **x/vt is NOT adopted for this
  engine** — the P0 exit gate (§15 item 7) **BLOCKS adoption** and we fall to the pre-vetted
  `midterm` drop-in (§5.4), re-running the spike against it. (A missing visible-grid signal
  (i-b) — no `RowWrapped` and a `Render()` that hard-breaks wrapped rows — likewise blocks.)
  This is consistent with the locked "adopt behind interface; `midterm` is a pre-vetted DROP-IN
  swap" decision: the interface seam exists precisely so a missing capability triggers a backend
  swap, not a half-correct reconstruction inside our adapter. Because the evicted-row flag is
  delivered by the hook *at eviction time* and the visible-row flag is read live at paint, a
  region-internal scroll, a width change, or a multi-screen flood needs **no** side-array
  bookkeeping to stay correct — the `wrapped` bit `drainEvictedLine` / `encodeRow` consume is
  whatever the backend reports for that exact row at that exact moment.
- **x/vt is constructed with `WithScrollback(0)` — ZERO retained cell scrollback.** The
  eviction hook captures each line before x/vt would drop it, so x/vt needs no window of its own.
  x/vt's retained cell cost is therefore **~0, constant, independent of `scrollbackLines`** — and
  the compact ring is **not** a second copy of a deep scrollback; it is the only deep copy, and it
  is compact. (`vtScrollbackWindow` is the named constant `0`, §4.1.)

**Drain mechanism (resolved, single mechanism — blocker resolution).** There is **exactly one**
production mechanism: the **pre-scroll / eviction hook**, a HARD P0 adoption gate.

- **The hook (the only mechanism).** x/vt fires a `Callbacks` entry (or a thin wrapper around its
  scroll path) for **each** line that scrolls off the top of the primary grid, **before** x/vt
  drops it, handing the adapter the leaving row's `uv.Cell`s *and* its `wrapped` bit;
  `drainEvictedLine` encodes it once (§9.2.1) into the compact ring. x/vt is built with
  `WithScrollback(0)`, so its retained-cell cost is ~0. Because the hook fires **per line, in
  stream order, synchronously inside `emu.Write`/`emu.Resize`**, no single `Write` can outrun it:
  a restore redraw replaying the full saved depth, a multi-screen 64 KB flood, or a `CSI Ps S`
  that scrolls thousands of lines at once all deliver **every** evicted line to the hook in order.
- **Why the `ScrollbackLen()`-diff fallback is DELETED (the per-`Write` capture ceiling).** The
  previously-listed fallback diffed `ScrollbackLen()` *after* `emu.Write` and read evicted lines
  via `ScrollbackCellAt` from a small retained window. But x/vt's window retains only the
  **most-recent ~`vtScrollbackWindow` (256)** evicted lines *at the moment the diff runs* — so any
  single `Write` that scrolls **more than the window** drops the earlier evictions **before** the
  diff can read them, silently punching holes in the authoritative compact ring. This is **not** a
  constant-factor difference from the hook: the hook has **no** per-`Write` ceiling, the diff has
  a **hard** one. Two concrete manifestations made this lossy on accepted paths: (1) RESTORE feeds
  the entire saved redraw (up to `scrollbackLines` = 10000 step-5 lines) in **one** `model.Write`,
  so the diff would reconstruct only ~256 lines instead of 10000 — failing
  `TestRegression_RoundTripScrollbackDepth` and violating G3/G5; (2) a live 64 KB flood evicts
  thousands of lines in one `pumpStep` `model.Write`. **No cheap per-`Write` sub-batching bound
  fixes this** — a 1-byte `LF` evicts one line and a few-byte `CSI 99999 S` evicts ~99999, so byte
  count does not bound eviction count without re-parsing the stream (the very thing the adapter
  refuses to do). The diff fallback is therefore removed entirely; the hook is mandatory.

The stored **depth** is the compact ring (O(`scrollbackLines`) compact bytes); x/vt's retained
**cell** cost is **~0** (`WithScrollback(0)`), never O(`scrollbackLines`). The §9.3 budget and
§9.4 `ModelBytes()` therefore have a **single** outcome (retained window = 0), not two. The P0
exit gate (§15 item 7) confirms the hook is present **before** any `model/` code is built; if it
is absent, adoption blocks (→ `midterm`), it does not silently fall back to a lossy diff.

**Alt-screen drain guard (the data-corruption fix — blocker Issue).** `drainEvictedLine(cells,
wrapped)` (the hook handler) is a **no-op while `shadow.altScreen` is true** — its first statement
is `if m.shadow.altScreen { return }`, applied on **both** call sites (the `Write` and `Resize`
eviction hooks). The authoritative compact ring (`m.scrollback` / `m.sbBytes`) is the **PRIMARY**
buffer's scrollback *only*. The alt buffer has no scrollback, so any line displaced off the top of
the alt grid — whether by an alt-internal scroll **or by a height-SHRINK while a full-screen app
owns the alt buffer** (the listed "resize during alt-screen" edge case) — MUST NOT be encoded into
the primary ring. Without the guard, the `WithScrollback(0)` eviction hook would fire for those
displaced **alt** rows and append them to `m.scrollback`, permanently polluting the primary
scrollback that is serialized (step 5) *after* the app later exits (`?1049l`) — a silent corruption
on the common "resize a pane while vim/htop is open" path. The guard makes `m.scrollback` and
`m.sbBytes` provably **invariant for the entire duration the session is in the alt buffer**. (When
the app exits, §11.1's
`OnForegroundReset` / a real `?1049l` flips `shadow.altScreen` back to `false` *before* the next
drain, and x/vt's getters return the restored primary grid, so primary-row eviction resumes
normally.) Asserted by a `model/` unit test and `TestRegression_ResizeDuringAltScreenNoPollution`
(§13.1/§13.3): a height-shrink in the alt buffer leaves the primary compact ring + `sbBytes`
**unchanged**, and a later `?1049l` + serialize yields a clean, uncorrupted primary.

**Width-change resize — no stale flag possible (major Issue, settled by the blocker
resolution).** Because there is **no adapter-maintained wrap side array** and every row's
`wrapped` bit is delivered by x/vt at eviction (the hook) or read live at paint (`RowWrapped`), a
width change carries **no width-specific flag forward to go stale** — the eviction hook stamps each
displaced row with x/vt's per-row signal at the new width. Whether x/vt **reflows the LIVE visible
grid** on a width change (vs clip/extend) is a §15 P0 spike question, but it is now an
**x/vt-internal** concern: whatever wrap state x/vt settles on, our capture reports it faithfully
because it comes from x/vt, not a cached side array. Asserted by the §13.1
width-shrink/grow-then-evict unit test (the rows evicted after the resize carry x/vt's
post-resize wrap flags).

**Partial-region (DECSTBM) drain guard (major Issue).** `drainEvictedLine` early-returns
when the scroll that produced the eviction occurred **inside an active partial scroll region**
(`shadow.scrollRegionSet && !(scrollTop==1 && scrollBottom==rows)`): a line scrolled off the top of
a partial region is **discarded by the terminal, never added to scrollback**, so capturing it would
corrupt the compact ring. With no side array there is **no shift to perform** — the only
question is whether to *append*, and the answer is "no" for a region-internal scroll. The
*append* (capture) runs **only** for full-screen scrolls and full-screen height-shrinks. §15 P0
confirms x/vt pushes to scrollback **only** when the region spans the full screen — i.e. the
eviction hook **fires only** for a full-screen scroll/shrink; the handler's full-screen condition
is a defensive double-check on top of that. Any visible row whose wrap state changes inside the
region is reported correctly on the next paint because `RowWrapped` reads x/vt live. Asserted by `TestRegression_PartialScrollRegionNoDrain` (§13.3)
and a `model/` unit test driving both a full-screen and a partial DECSTBM region.

### 9.2.1 The per-row compact-ANSI encoder (specified, and byte-identity-gated)

Both the authoritative scrollback ring AND the §6 step-5 / step-7 (β) paint depend on **one**
function that turns a single evicted/visible row of `uv.Cell`s into compact clean ANSI. The
`.buf` self-fixpoint (`TestRegression_SerializeFixpointThroughXVT`, gated at P0) requires this
per-row encoding to be **byte-identical** to what the grid `Render()` emits for the
corresponding row, so the encoder is specified concretely — not left to the implementer:

```go
// encodeRow renders one terminal row to compact, ground-state-terminated ANSI.
//   cells   : the row's uv.Cells (from the eviction hook for scrollback, or grid CellAt for the visible grid).
//   wrapped : true iff this physical row soft-wraps into the next (the row's isWrapped attr).
// Returns the exact bytes step 5 stores/emits for the row. Deterministic (canonical SGR).
func encodeRow(cells []uv.Cell, wrapped bool) []byte
```

- **Primary mechanism — reuse ultraviolet's own single-row render.** ultraviolet renders the
  grid row-by-row; we build a **1-row `uv.Buffer`** from `cells` (same `uv.Cell` type as
  x/vt — zero conversion) and render it through the **same ultraviolet single-line entrypoint
  `Render()` uses internally per grid row**. The per-row bytes are then **by construction
  byte-identical** to the grid `Render()`'s output for that row — this is what makes the
  fixpoint hold. Whether ultraviolet exposes a public single-row render entrypoint is a **P0
  capability question** (§15).
- **Fallback — hand-rolled canonical SGR-RLE.** If ultraviolet exposes no single-row render
  API, `encodeRow` is a hand-rolled left-to-right SGR run-length encoder that emits an
  `ESC[…m` SGR sequence **only when the pen changes** between consecutive cells, in
  ultraviolet's **canonical SGR attribute ordering** (the same canonical form §13.1 already
  mandates for determinism), writes each cell's glyph(s) (incl. combining marks; wide glyphs
  consume their trailing placeholder cell), and ends the row in ground state. **The P0 exit
  gate (§15 item 2) REQUIRES the hand-rolled output to be byte-identical to the grid-row
  `Render()` output** on the scrollback fixture; if it cannot be made identical at the pinned
  commit, that is a P0 exit-gate failure that blocks adoption (→ evaluate `midterm`, §5.4) —
  the §13.3 fallback (verbatim reuse of the immutable stored compact bytes for the
  *unchanged* scrollback region) then carries the scrollback half, but the **visible-grid**
  row encoding must still self-fixpoint or adoption is blocked.
- **Trailing-blank trimming (wrap-aware, spacer-aware).** A **non-wrapped** row trims trailing
  default-attribute blank cells before encoding (matching `SerializeAddon`, smaller bytes). A
  **wrapped** row is encoded at **full `cols` width with NO trimming**, so the continuation
  triggers autowrap at exactly the right column in the target xterm. `Render()`'s own
  per-row output follows the same rule (P0 confirms), preserving the byte-identity above.
- **Wide glyph at the right margin (the CJK-spacer rule, minor Issue).** A double-width glyph
  that cannot fit in the final column leaves a **blank spacer cell at column `cols`** and the
  glyph itself wraps to the next row (xterm/`SerializeAddon` behavior). That spacer is **not** a
  trailing default blank — it is a **load-bearing wrap artifact**, and the row that carries it is
  inherently a **wrapped** row (the glyph continues below). The encodeRow rule is therefore exact:
  **(1)** a row whose last occupied column is a right-margin wide-glyph spacer is treated as
  `wrapped == true` and encoded at **full `cols` width with NO trimming** (the spacer is emitted as
  its blank cell), so the deferred wide glyph re-wraps at the identical column in the target xterm;
  **(2)** for a non-wrapped row, trailing-blank trimming **stops at the last cell that is either
  non-default OR a wide-glyph spacer** — i.e. trimming never swallows a spacer, only genuine
  default blanks beyond it. This matches what the grid `Render()` emits for the same row (P0
  confirms), preserving the byte-identity above for CJK content straddling the margin. Asserted by
  a `model/` unit case (`TestEncodeRow_WideGlyphRightMargin`, §13.1) that builds a row with a CJK
  glyph straddling the right margin and asserts `encodeRow` is **byte-identical** to the grid
  `Render()` for that row, plus the §13.2 `widewrap`/CJK fixtures.

The same `encodeRow` is used (i) on eviction to fill the `lineRing`, (ii) by §6 step 5 to
emit stored scrollback, and (iii) by §6 step 7 in the **β** branch to paint the visible grid
rows when `Render()` lacks the required `CR`/`LF` shape — one encoder, three call sites,
provably fixpoint-stable.

### 9.2.2 The `lineRing` type (the authoritative compact-scrollback structure)

`lineRing` is the daemon's authoritative full-depth scrollback (§9.2): a fixed-capacity FIFO
of compact pre-serialized evicted rows. It is referenced pervasively — `newLineRing`, append
on eviction, ordered oldest→newest iteration for step 5, `Len()`/`ScrollbackLen()` parity,
O(1) `sbBytes` maintenance, `Cap()` for the §12 header and `HeaderState()` — so its type and
method surface are specified to the same precision as `encodeRow`/`shadowState`, not left to
the implementer. It is an unexported type in package `model`; the session never touches it.

```go
// lineEntry is one evicted physical row: its compact clean-ANSI bytes (from encodeRow,
// §9.2.1) and its soft-wrap flag (true ⇒ the logical line continues on the next stored row;
// gates step 5's \r\n separator). Bytes are immutable once appended — rows are never reflowed
// (§10(a)), so encode-once-on-eviction is correct.
type lineEntry struct {
	bytes   []byte
	wrapped bool
}

// lineRing is a bounded FIFO ring of lineEntry, cap = scrollbackLines (§9.1). When full, an
// Append evicts the OLDEST entry (chronological FIFO; oldest scrolled furthest into history).
// It maintains a running byte sum so ModelBytes() (§9.4) is O(1), and exposes an ordered
// oldest→newest iterator for step 5. NOT goroutine-safe: every access is under the owning
// session's s.mu (§8), identical to the rest of the adapter.
type lineRing struct {
	buf     []lineEntry // backing store, len == cap once warmed (ring indices wrap)
	head    int         // index of the OLDEST entry
	n       int         // count of live entries, 0 ≤ n ≤ cap
	cap     int         // == scrollbackLines; the configured depth
	byteSum int64       // running Σ len(entry.bytes) over the n live entries
}

// newLineRing builds an empty ring of the given depth. A non-positive cap is resolved to the
// default via resolveScrollback (§9.1) by the caller (newVTModel), so cap is always ≥ 1 here.
func newLineRing(cap int) *lineRing

// Append adds one evicted row. If the ring is full (n == cap) it FIRST evicts the oldest
// entry — decrementing byteSum by exactly len(evicted.bytes) (the O(1) decrement the §9.4
// accounting depends on) and advancing head — THEN stores the new entry at the tail and adds
// len(bytes) to byteSum. So byteSum is always the exact sum of the live entries' byte lengths,
// maintained O(1) on BOTH the append and the cap-eviction. Callers pass the encodeRow output
// and the row's wrap flag (§9.2 "Wrap-flag source").
func (r *lineRing) Append(bytes []byte, wrapped bool)

// Len returns the number of stored entries (n). This is the value §13.3's RoundTripScrollbackDepth
// asserts equals the reconstructed xterm's scrollback line-count, and that step 7's grid must
// extend the flow to exactly Len()+rows physical lines (§6 step 7).
func (r *lineRing) Len() int

// Cap returns the configured depth (scrollbackLines). Sourced by HeaderState() (§4.1) for the
// §12 CRWB1 header so a restore rebuilds the ring at the identical depth.
func (r *lineRing) Cap() int

// Bytes returns byteSum — the O(1)-maintained running sum used by vtModel for the sbBytes term
// of ModelBytes() (§9.4). (vtModel mirrors this into its sbBytes field on each Append, or reads
// it here directly; either way the value is O(1), never an O(depth) re-sum.)
func (r *lineRing) Bytes() int64

// Each iterates the live entries in CHRONOLOGICAL order — OLDEST (head) → NEWEST (tail) — which
// is exactly the order §6 step 5 emits scrollback (earliest history first, so it scrolls
// furthest up). The callback receives each entry's (bytes, wrapped); iteration allocates
// nothing and mutates nothing.
func (r *lineRing) Each(fn func(bytes []byte, wrapped bool))
```

`vtModel.scrollback` is a `*lineRing`; `vtModel.sbBytes` is kept equal to
`scrollback.Bytes()` (updated on each `Append`), so `ModelBytes()` (§9.4) reads it O(1).
`drainEvictedLine` (the eviction hook, §9.2) is the sole `Append` caller; step 5 is the sole `Each` caller;
`HeaderState()` is the sole `Cap()` caller. A `model/` unit test asserts the O(1) byte-sum
decrement on cap-eviction (append past `cap`, assert `Bytes()` equals the sum of the surviving
entries and `Len() == cap`) and the oldest→newest iteration order.

### 9.3 Memory budget math (single outcome — x/vt retains no scrollback)

Per live session:

- **Live grid:** `cols × rows` cells. At 200 × 50 × ~16 B = **~160 KB** (bounded, fixed,
  reused — does not grow).
- **x/vt retained cell scrollback:** **~0.** x/vt is built with `WithScrollback(0)` (the
  mandatory eviction hook captures each line before x/vt would keep it — §9.2), so
  `vtScrollbackWindow × cols × bytesPerCell == 0`. There is no longer a "diff-fallback vs
  hook" fork in the budget: it is a **single** outcome. (`ModelBytes()` still counts this
  term explicitly — it is just 0.)
- **Compact-line ring (authoritative scrollback):** `scrollbackLines` compact lines,
  worst-realistic ~80 B/line × 10000 = **~800 KB**; typical far less (most lines are
  short / blank and stored compactly).
- **Cached `lastBlob` (§8.4 cadence-flush cache):** the last serialized blob, worst-case
  ≈ the compact scrollback + grid ≈ **up to ~800 KB**; typical far less. It is counted in
  `ModelBytes()` (§9.4), so the ceiling cannot be undercounted by it — but it is a
  **reclaimable cache** (see the drop-under-pressure rule below), not a hard resident cost.
- **Shadow + emulator overhead:** a few KB.

So the **gross** per-session worst case is **~1.8 MB** once the `lastBlob` term is included
(live grid + compact ring + cached blob; the x/vt window term is 0) — at 100 sessions a
**gross** peak of **~180 MB**, comfortably under the 256 MiB `maxTotalModelBytes` ceiling.
`lastBlob` is still not treated as fixed: **`lastBlob` is the FIRST thing the maintenance loop
reclaims under memory pressure** (§9.4) — dropping it is correctness-free (it is reconstructible
by re-serializing the model on the next `Snapshot()`), so the **enforced resident floor** after
reclaiming the aggregate ~80 MB of cached blobs is **~100 MB**, the same order as today's 1 MiB
raw ring ceiling, with far higher fidelity per byte. Because the drain mechanism is now a single
outcome, the budget no longer depends on a spike result; `ModelBytes()` (§9.4) counts the (zero)
x/vt window term **and** `lastBlob`, so the maintenance ceiling enforces against *true resident
memory* while the reclaim order keeps the cache from forcing spurious suspends.

### 9.4 Replacing RingCap/Stats memory accounting

`RingBuffer.Cap()` and `Session.RingCap()` are gone. The engine's memory accounting
(`Stats()`, `maxTotalRingBytes`, the `underCeiling()` closure in `runMaintenanceOnce`)
switch from "ring capacity bytes" to **model resident bytes**:

- Add `Session.ModelBytes() int64` returning the session's estimated resident size,
  **including x/vt's retained cell window AND the cached last-blob** so the ceiling is not
  undercounted: `m.ModelBytes() + len(s.lastBlob)`, where the model term is
  `(cols*rows*bytesPerCell) + (vtScrollbackWindow*cols*bytesPerCell) + sbBytes` — the middle
  term is x/vt's retained cell scrollback, which is **0** because `vtScrollbackWindow == 0`
  (x/vt built with `WithScrollback(0)`; the eviction hook is the sole capture path, §9.2), and
  `sbBytes` is the compact ring's running byte sum (maintained O(1) on append/evict) — and
  `len(s.lastBlob)` is the §8.4 cached serialized blob the cadence-flush skip retains. The
  `vtModel` computes its term from `Cols()*Rows()`, the (zero) window, and `m.sbBytes`; the
  `Session` adds `len(s.lastBlob)`; cheap, O(1) amortized. (The window term is kept in the
  formula literally so a future backend that *did* retain a cell window would account it.)
- `Session.RingCap()` → **renamed** `Session.ModelBytes()`. All call sites in `terminal.go`
  (`Stats`, `underCeiling`, the Phase-3 ceiling logic) call the new method. **Suspended
  placeholders build NO `vtModel`** (§4.2): they hold only `s.rawBlob` (the named placeholder
  field, §8.4), so `ModelBytes()` returns `len(s.rawBlob)` (the live-grid + x/vt-window +
  `lastBlob` terms are zero — no live model and `s.lastBlob == nil`). This preserves today's
  lazily-sized-placeholder footprint and
  is precisely why thousands of placeholders do not each pin ~0.8–1.8 MB — the Phase-3c
  LRU eviction continues to bound the *blob* total, not a pile of full models.
- `maxTotalRingBytes` is **renamed** `maxTotalModelBytes` (same 256 MiB value, same
  semantics: a global resident-memory ceiling that the maintenance loop enforces by
  suspending/evicting). The behavior of Phases 2/3 is otherwise unchanged.
- **`lastBlob` is reclaimed FIRST under memory pressure — the concrete method + maintenance
  phase (Issue: the reclaim was prose-only).** Because `ModelBytes()` **counts** `len(s.lastBlob)`
  (above), the 256 MiB ceiling can be tripped **purely by reclaimable cache** — so the ordering
  of the reclaim relative to suspend/evict is **correctness-relevant, not cosmetic**: without it,
  `underCeiling()` (which now sums the caches) would drive spurious suspend/evict churn the moment
  caches fill. The reclaim is pinned exactly like the rest of the spec:
  - **The `Session` method.** Add `func (s *Session) DropCachedBlob()` which, under `s.mu`,
    sets `s.lastBlob = nil` **and** `s.dirty = true` (so the next `Snapshot()` re-serializes a
    correct, current blob — dropping the cache is correctness-free). It is a no-op for a
    placeholder (`s.model == nil`, `s.lastBlob` already nil) and idempotent. It returns the number
    of bytes reclaimed (the old `len(s.lastBlob)`) so the sweep can stop as soon as it is under.
  - **The new maintenance phase — "Phase 3 pre-step," inserted BEFORE Phase 3a.** In
    `runMaintenanceOnce` (terminal.go), immediately **after** the `underCeiling()` early-return at
    the top of Phase 3 (so it runs only when actually over the ceiling) and **before** Phase 3a's
    idle-suspend: iterate the **live** sessions (`s.model != nil` with a non-nil `lastBlob`)
    **LRU-coldest-first** (the same `getLastActive` ordering Phases 3a/3b/3c already use), calling
    `DropCachedBlob()` on each and re-checking `underCeiling()` after each drop; **return early the
    instant `underCeiling()` holds.** Only if dropping **every** live session's cache still leaves
    the total over the ceiling does control fall through to the **existing** Phase 3a (idle
    suspend), 3b (force-suspend detached), and 3c (evict placeholders) — i.e. **no session is
    suspended or evicted while any reclaimable cache remains.** Because `DropCachedBlob` sets
    `dirty`, an idle session whose cache is reclaimed simply re-serializes on its next genuine
    flush; it does **not** spuriously mark the session for suspend.
  - This reclaims the aggregate ~80 MB of cached blobs (§9.3) cheaply and reversibly, so a full
    cache never triggers spurious suspend/evict churn under the ceiling.
    `TestRegression_IdleSessionNoCadenceWrite` is unaffected (an idle session that kept its
    `lastBlob` still skips the disk write via `changed`); and
    `TestRegression_CacheDropReclaimsBeforeSuspend` (§13.3) drives the global total over the
    ceiling with reclaimable caches and asserts the Phase-3 pre-step **drops `lastBlob` on the
    coldest live sessions and returns under the ceiling with ZERO `Suspend`/`evict` calls**, then
    (a paired case) that when caches alone are insufficient the loop *does* proceed to 3a/3b/3c.
- **`Stats()` signature change + parse-health surface (Issue — under-specified vs the §9.1
  Create-plumbing bar; corrects the false "already exposes" claim).** The session-level
  `s.modelPanics` counter is **NEW in this spec** (§8.2, added to `Session` by this change) —
  it does **not** exist in `session.go` today, so "`Stats` already exposes it" is wrong; it is
  exposed **as part of this same change**, enumerated below to the §9.1 precision so the
  compile-breakage surface is not rediscovered piecemeal.

  **New `Stats()` signature** (the renamed bytes field plus two new health fields):

  ```go
  // BEFORE (terminal.go:178 interface decl AND terminal.go:1085 impl):
  Stats() (active, detached, suspended int, ringBytes int64)
  // AFTER:
  Stats() (active, detached, suspended int, modelBytes int64, degraded, parsePanics int)
  ```

  - `ringBytes int64` is **renamed** `modelBytes int64` (the §9.4 RingCap→ModelBytes rename:
    the loop body changes from `ringBytes += int64(s.RingCap())` to
    `modelBytes += s.ModelBytes()` at **terminal.go:1094**, and the Phase-3 ceiling sum at
    **terminal.go:1302** changes the same way; both already covered by the RingCap→ModelBytes
    bullet above — the field is renamed in lock-step).
  - `degraded int` is the **count of currently-degraded live sessions** and `parsePanics int`
    the **summed recovered-panic count** across all sessions. They are accumulated in the
    same `Stats()` loop, per session, via the optional `model.ModelHealth` assert (§4) — **no
    concrete `*vtModel` downcast**:
    ```go
    if h, ok := s.model.(model.ModelHealth); ok {
        if h.Degraded() { degraded++ }
        parsePanics += h.ParsePanics()
    }
    ```
    A placeholder (`s.model == nil`) or a backend that does not implement `ModelHealth`
    contributes `0` to both — i.e. it reports `degraded`-clean. This is the sanctioned path
    that makes "adapter degraded surfaced in Stats" (§3.3/§4.1) true without widening the
    minimal `TerminalModel` interface. (The per-session `s.modelPanics` field, bumped by the
    §8.2 defense-in-depth backstop, remains the session-local mirror; `parsePanics` is its
    cross-session aggregate sourced through `ModelHealth.ParsePanics()`.)
  - **Call sites that MUST change** (the compile-breakage surface, enumerated like §9.1
    3a–3e): the two-arg additions are mechanical `_` placeholders in the test destructures —
    - `terminal.go:178` — interface method decl → new signature.
    - `terminal.go:1085` — impl signature → new signature; the loop gains the `ModelHealth`
      assert above and the `ringBytes`→`modelBytes` rename.
    - `bounds_test.go:45` — `active, detached, suspended, ringBytes := eng.Stats()` →
      `active, detached, suspended, modelBytes, _, _ := eng.Stats()`; the
      `bounds_test.go:48` assertion variable renames `ringBytes`→`modelBytes`.
    - `maintenance_test.go:357` — `_, _, _, ringBytes := eng.Stats()` →
      `_, _, _, modelBytes, _, _ := eng.Stats()` (and the `:358` `SetMaxTotalRingBytesForTest`
      arg uses `modelBytes`).
    - `maintenance_test.go:456` — `a, d, s, rb := eng.Stats()` → `a, d, s, rb, _, _ := eng.Stats()`.
    - `maintenance_test.go:464` — `_, det, _, ringB := eng.Stats()` → `_, det, _, ringB, _, _ := eng.Stats()`.
    - `maintenance_test.go:483` — `_, detAfter, susp, _ := eng.Stats()` →
      `_, detAfter, susp, _, _, _ := eng.Stats()`.
    A new `maintenance_test.go` case asserts the two health return values: after a forced
    recovered `model.Write` panic on one live session, `Stats()` reports `degraded == 1` and
    `parsePanics >= 1`, and a clean engine reports `0, 0`.

---

## 10. Resize, alt-screen, flood (the three resolved open points)

### (a) Resize

- On a client resize message, `readPump` calls `s.Resize(cols, rows)`: under **one `s.mu`
  hold** it calls `pty.Setsize` (→ `SIGWINCH`) **then** `s.model.Resize(cols, rows)`, with
  no intervening `model.Write`, so the model grid tracks the PTY in lockstep (§4.2). This
  intentionally holds `s.mu` across the `Setsize` ioctl (a change from the old
  release-before-ioctl behavior); the hold is bounded and accepted in §8.3. The app
  re-renders at the new size on the next frame, which the model parses normally.
- **Resize is a persistence-relevant mutation: it sets `s.dirty = true` and clears
  `s.lastBlob` (cache invalidation) in that SAME `s.mu` hold (§4.2 Issue).** A resize emits
  no PTY output, so absent this the §8.4 cadence cache would persist the **pre-resize** blob
  (`changed == false`) and the `CRWB1` header would keep the stale `cols×rows`, corrupting a
  later restore. With the dirty mark + cache clear, the next `Snapshot()` (cadence flush,
  detach, suspend, or attach) re-serializes at the new size and persists a new-size header.
- **Serialize uses the model's current size.** The redraw reflects whatever size the model
  is at when the new client attaches; the client's `xterm` is created at its own size and
  the app self-corrects on its next frame (TUIs repaint on `SIGWINCH`; shells redraw the
  prompt).
- **Re-attach at a different width than the detached model — accepted scrollback residual
  (Issue, same no-reflow family as NG2/§6.2).** On a plain re-attach (workspace switch-and-
  back), unlike daemon-restart restore, there is **no header/resize-from-header path**: the
  daemon serializes at the **detached model's** size while the re-attaching client builds a
  fresh `xterm` at its **own** current pane size. If the user resized the window while the
  terminal was detached in another workspace, those differ, and the §6 redraw is processed
  before the client's first resize round-trips. We deliberately do **NOT** drive a client
  resize-to-model-size around the redraw (it would fight the client's real pane geometry and
  flicker). The consequences are split and both bounded:
  - The **visible grid** self-heals: the app repaints at the new size on the `SIGWINCH` the
    re-attach implies (TUIs unconditionally, shells re-emit the prompt), exactly as the
    "Serialize uses the model's current size" bullet states.
  - The **step-5 scrollback rows** are encoded full-width relying on autowrap at the model's
    `cols` (§9.2.1). In a narrower/wider client `xterm` those historical lines **mis-wrap**,
    and — because scrollback is neither redrawn by the app nor reflowed by us (NG2) — they
    stay mis-wrapped **until evicted** by new output. This is an **accepted bound**, in the
    same no-reflow family as NG2 and §6.2: G3's identical-buffer guarantee is explicitly
    scoped to a *fresh same-size* `xterm` (§6.1/§13.2), so a different-width re-attach is
    outside the guaranteed envelope by design. It is stated here as an asserted, bounded fact
    — mis-wrapped historical lines until eviction, visible grid self-heals — rather than an
    unspecified divergence. `TestRegression_ReattachAtDifferentWidthScrollbackResidual`
    (§13.3) re-attaches at a narrower width and asserts the **visible grid** matches the model
    (post-repaint) while documenting the scrollback mis-wrap as the bounded, non-corrupting
    residual (no dangling sequence, no query echo — only physical wrap of historical rows).
- **Stored scrollback is NOT reflowed.** Lines serialized at an old width stay at that
  width. This matches `tmux`, iTerm2's default, and most terminals — reflow is expensive,
  lossy for wrapped wide glyphs, and unnecessary for the re-attach use case. (Non-goal NG2.)
- **Resize captures height-shrink evictions into the compact ring (explicit).** A height
  **shrink** makes the top grid rows overflow; a VT emulator pushes them off the top exactly
  as a scroll does, so x/vt fires the **same eviction hook** (`drainEvictedLine`) for them
  **during `emu.Resize`** (§4.1) — encoding each once (§9.2.1) into the authoritative compact
  ring with the correct line-count and `sbBytes`. There is no post-resize diff pass and no
  retained window for a row to "age past" (x/vt is `WithScrollback(0)`); the hook captures every
  shrink-displaced row in order. **The hook handler is guarded `!shadow.altScreen` (§9.2
  "Alt-screen drain guard"): a height-shrink while a full-screen app owns the alt buffer
  displaces ALT rows, which must NEVER enter the primary compact ring — so a resize during
  alt-screen leaves the primary scrollback + `sbBytes` provably unchanged, and a later `?1049l`
  + serialize yields a clean primary (`TestRegression_ResizeDuringAltScreenNoPollution`,
  §13.3).** A height **grow** does the opposite of reflow: it does **NOT** pull stored compact
  lines back onto the grid. With `WithScrollback(0)` x/vt has no retained window to repopulate
  from, so a grow simply adds blank rows at the bottom — identical to `tmux` — and the eviction
  hook does not fire for a grow (nothing scrolls off the top), so there is **no** grow-boundary
  duplication to reconcile. `TestRegression`/`model` unit tests assert a height-shrink that
  overflows the grid captures the displaced rows into the ring with the correct count + `sbBytes`,
  and that a subsequent grow neither un-captures nor double-counts. **§15 P0 confirms x/vt fires
  the eviction hook for shrink-displaced rows; if a pinned commit's shrink path bypasses the hook,
  a thin wrapper around `emu.Resize` re-routes the displaced rows through `drainEvictedLine`, else
  adoption blocks → midterm.**
- **A WIDTH change needs no wrap-flag fix-up — the flag is read live from x/vt (§9.2
  "Width-change resize" / §4.1).** Stored scrollback is still not reflowed (NG2) —
  already-evicted compact lines are immutable, and each was stamped with the wrap flag x/vt
  reported **at its own eviction time** (never recomputed afterward, so never stale). The
  **live** grid's per-row wrap state is whatever x/vt settles on after the resize (whether x/vt
  re-wraps the live grid vs clip/extend is a §15 P0 question, but an x/vt-internal one): the
  next paint reads it via `RowWrapped`, and the next eviction's flag is delivered by the hook,
  so a row evicted shortly after a width change is stamped with x/vt's **current** flag, never a
  stale cached one. There is no adapter side array to recompute
  or reset — the blocker resolution (§9.2) removed it precisely because the adapter cannot
  reconstruct wrap state from chunk-boundary getters. Asserted by the §13.1
  width-shrink/grow-then-evict unit test.

### (b) Alt-screen re-attach

The §6 sequence is **one conditional sequence**, not two; alt-screen simply selects
which branches fire. When `shadow.altScreen` is true at attach, `Serialize`:

1. emits `?1049h` in **step 3** (enter the alt buffer), then
2. **skips step 5 (scrollback) entirely** — the alt buffer has no scrollback, and
   emitting `\r\n` lines into it would scroll the alt grid *and* corrupt the primary
   buffer that `?1049` saved (xterm restores that primary buffer on the app's later
   `?1049l`, so any garbage we scroll in now would surface as junk after vim/htop exit),
   then
3. emits the **active (alt) grid** via `Render()` in **step 7**, painting from `(1,1)`
   (step 4 already homed and step 5 was skipped, so step 6's "no re-home" is a true
   no-op here — the grid lands at the top of the alt buffer), then
4. asserts origin mode in **step 12** (a no-op for the common alt app, which rarely sets
   DECOM) and positions the cursor in **step 13**, then runs the rest of the epilogue normally.

So the alt-screen attach payload is exactly steps **1, 2, 3, 4, 7, 8–17** with **step
5 omitted** (and step 6 the no-op it always is) — fully determined by `shadow.altScreen`,
with no separate code path. The
fast full-screen app (vim/htop/less) **self-corrects on its next frame** — it owns the
alt screen and repaints unconditionally on redraw.

**We do NOT restore the hidden *primary* screen behind the alt buffer — and the
consequence is a real, accepted data loss, stated honestly here (the G3 carve-out).**
Step 3's `?1049h` is fed into the new xterm whose primary buffer is **empty** (it was just
created), so xterm *saves an empty primary*. If the alt app later exits and emits `?1049l`
(raw passthrough on the live stream), xterm switches back to that **empty** primary: the
pre-app shell screen and scrollback that the original terminal had before the app launched
are **gone** for this re-attached client. **This is the one fidelity gap in the design.**
We accept it because the locked decision is to serialize only the active alt grid (x/vt
does not expose the inactive primary buffer for us to re-paint underneath, and re-architecting
to do so is explicitly out of scope), the loss is bounded to the alt-reattach-then-exit
corner, and the post-exit primary is **clean-empty, never corrupted**.

**Correction of a prior false claim:** earlier drafts asserted "the app restores the
primary on `?1049l`, matching `tmux`." That is **wrong** and is removed: `tmux` re-paints
the primary from *its own* model after `?1049l`, whereas we have no primary-buffer model to
re-paint from, so the new xterm restores an empty primary. The only thing that legitimately
"matches `tmux`" in the alt branch is that the *alt app itself* repaints its alt content on
its next frame; the primary-restoration claim does not hold and must not be relied on.

This reconciles §6 step 5's "only if `!altScreen`" condition: emitting `\r\n` scrollback
lines into the alt buffer would scroll the alt grid *and* pollute the saved (empty) primary,
which would then surface as junk after the app exits — strictly worse than a clean-empty
primary. So scrollback is omitted in the alt branch by design, and the bounded loss above is
the documented price. `TestRegression_ReattachDuringAltScreenThenExit` (§13.3) pins this as
an asserted, non-corrupting outcome.

### (c) Flood

- The live path is **raw-first fan-out** (§8.2): a flooding app's bytes reach clients via
  `fanOutLocked` *before* the model is touched. The model update is decoupled and
  **never backpressures** live output.
- `writePump`'s existing coalescing (`maxCoalesceBytes`, UTF-8 holdback) is unchanged and
  continues to collapse bursts.
- The model parses one extra time per chunk — pure CPU, bounded, and **synchronous under
  the same `s.mu` as the fan-out** (§8.2). It therefore **cannot lag** the live stream:
  after `pumpStep` returns, the model is exactly as current as the last fanned-out byte
  (§8.3). There is no "model falls behind / stale serialize" scenario — that was an
  artifact of an earlier draft and is impossible under the single-lock design. The only
  cost a flood imposes is the bounded `s.mu` hold time quantified in §8.3 (sub-millisecond
  per screenful chunk, dominated by per-evicted-line compact encoding), which delays
  *contending* client `Write`/`Resize`/`Attach` by that much but never the live *output*,
  which is enqueued before the model is touched. If we ever wanted to shrink that hold by
  decoupling `model.Write` to a goroutine, §8.3's atomicity and this no-lag guarantee
  would both have to be re-proven first — so the synchronous design is intentional, not
  incidental.

---

## 11. What is deleted vs kept

### Deleted

| Item | Why |
|---|---|
| `internal/session/replay_sanitize.go` (all regexes + `sanitizeReplaySnapshot`) | Serialized state contains no queries / historical OSC to strip. |
| `internal/session/dec_modes.go` (`decModeTracker`, `trackedDECModes`, `observe`, `preamble`) | The model tracks modes natively; `Serialize` re-asserts them with correct per-mode default polarity (steps 3/8/9/13). |
| The `snap = append(snap, 0x18)` CAN byte in `Attach`. | Serialized state is self-contained and ground-state-terminated; nothing can dangle. |
| `internal/session/ring.go` (`RingBuffer`, lazy-grow, `defaultRingSize`, `DefaultRingSize`). | Replaced by the model + compact-line scrollback. |
| `Session.RingCap()`, `maxTotalRingBytes` (name). | Replaced by `Session.ModelBytes()` / `maxTotalModelBytes`. |
| The `Attach` preamble block (`s.decModes.preamble()`), the `sanitizeReplaySnapshot` call. | Replaced by `serializer.Serialize(s.model)`. |
| Ring-specific tests: `ring_test.go`, `ring_bench_test.go`, `replay_sanitize_test.go`, `dec_modes_test.go`, and the ring/sanitize/dec-mode helpers in `export_test.go`. | Code under test is deleted. |

### Kept

| Item | Why |
|---|---|
| **FE one-shot repaint-finalize** (`use-terminal-connection.ts` `pendingAttachFinalizeRef` → `terminal.refresh(0, rows-1)`; `terminal-host.tsx` post-swap refit). | A **WKWebView renderer concern** (the WebGL canvas isn't repainted until invalidated), **orthogonal** to replay. It now fires on the first serialized-redraw flush instead of the first raw-replay flush — same trigger, still needed. **Do not remove.** |
| `pumpStep` lock discipline + `PumpChunkForTest` race test. | Still the production critical section; updated to fan-out-first + model-write. |
| `parseLastOSC7` + `s.cwd` tracking. | CWD is engine metadata (used for restore `cmd.Dir`), independent of screen modeling. Feeds `shadow.workingDir` too if `x/vt` lacks an OSC 7 callback. |
| `writePump` coalescing + UTF-8 holdback. | Transport-layer concern, unchanged. |
| The whole engine lifecycle (suspend/restore/reap, `sessionMu`, persistence flow, maintenance). | Unchanged except the payload form (raw bytes → serialized state) and the memory-accounting rename. |
| `creack/pty`. | PTY ownership is unchanged. |

### 11.1 Foreground-app death: clearing stale modes (replaces the deleted idle gate)

The deleted `Attach` preamble was gated on `!s.isIdleLocked()` for a real reason: a shell
that had returned to its prompt must NOT get mouse/alt-screen modes re-asserted into it.
Serialize-on-attach removes the *replay* but **not this hazard**. A foreground app that
enabled `?1049`/`?1000`/`?1002`/etc. and then **died without resetting them** (SIGKILL,
crash — no `?1049l`/`?1000l` is ever emitted) leaves `shadow.altScreen` and
`shadow.modes` stuck ON. A subsequent `Serialize` into the now-idle shell would emit
`?1049h`/`?1000h` (and serialize the stale alt grid), leaking alt-screen/mouse-tracking
into the prompt — *exactly* the symptom the idle gate prevented. We must keep an
equivalent, and we put it on the model's mode state rather than on a serialize-time gate.

**Mechanism — drive a model teardown off the foreground-process-group edge.** The session
keeps the same foreground-pgrp idle predicate the old gate used (`isIdleLocked()` /
`TIOCGPGRP` vs the shell's pgid) and tracks `lastForegroundPgid`. The detector is a small
helper `checkForegroundResetLocked()` (called under `s.mu`): it samples `TIOCGPGRP`,
compares to `lastForegroundPgid`, and when the foreground group **returns to the shell's
pgid** (the app exited, *by any means including SIGKILL*) fires `OnForegroundReset()`
**exactly once** for that edge (it latches on `lastForegroundPgid` so repeated samples in
the idle state do not re-fire). The `OnForegroundReset()` model call is invoked through
`s.mutateModelLocked(s.model.OnForegroundReset)` (§8.5) — because `checkForegroundResetLocked`
runs on three sampling sites that are all **outside** `writeModelLocked`'s recover (`pumpStep`
step (3), `Snapshot()`, and `runMaintenanceOnce`), the teardown's synthetic `model.Write` must
carry its **own** panic backstop so an emulator panic on the app-death edge bumps `modelPanics`
and continues rather than escaping to kill the pump/flush goroutine.

**Per-platform split (Issue — both builds must compile, reset semantics defined per
platform).** `TIOCGPGRP` is Unix-only, and the codebase already platform-splits idle
detection (`session_idle_unix.go` `//go:build !windows` with the real `TIOCGPGRP` ioctl vs
`session_idle_windows.go` `//go:build windows` returning a stub). `checkForegroundResetLocked`
is split the **same way**, mirroring the existing `isIdleLocked` pattern:
- **`session_foreground_unix.go` (`//go:build !windows`)** — the real implementation above:
  sample `TIOCGPGRP`, compare to `lastForegroundPgid`, fire `OnForegroundReset()` on the
  app→shell edge. (It reuses the same `TIOCGPGRP`/`Getpgid` machinery as the existing Unix
  `isIdleLocked`.)
- **`session_foreground_windows.go` (`//go:build windows`)** — a **no-op stub**:
  `func (s *Session) checkForegroundResetLocked() {}` (and `lastForegroundPgid` stays unused).
  ConPTY exposes no foreground process-group, so there is no app→shell edge to detect; the
  reset edge is therefore **defined as "never fires from this detector on Windows."** This
  matches the existing Windows `isIdleLocked` stub (`return false`) and keeps the three
  sampling sites below (which all call `checkForegroundResetLocked`) compiling and harmless on
  Windows — each call is a no-op. **Documented Windows residual (accepted):** a SIGKILLed
  alt-screen/mouse app that does not emit its own resets can leave stale transient modes until
  the next real app output re-establishes modes — the same limitation the pre-existing Windows
  idle stub already implies, and acceptable because the daemon's primary targets are
  macOS/Linux (the Tauri app ships on darwin). An app that exits *cleanly* (emits its own
  `?1049l`/`?1000l`) is unaffected on every platform, because those resets flow through
  `model.Write` and update the shadow regardless of the foreground detector.

**Sampling sites (pinned, not deferred).** `TIOCGPGRP` is a cheap ioctl (~single-digit
µs), but we keep it off the IO-free per-chunk hot path. It is sampled at exactly three
places, chosen so the reset is **guaranteed to have fired before any serialization**:

1. **Debounced in `pumpStep`, AFTER fan-out and model-write (hard ordering requirement).**
   `checkForegroundResetLocked()` runs at most once per **250 ms** of wall-clock per session
   (a `lastFgSampleAt` timestamp gate under `s.mu`), so a busy session converges promptly
   after an app dies but adds **at most ~4 ioctls/s**, not one per chunk. **It is the LAST
   statement in `pumpStep` — strictly after `fanOutLocked` and `writeModelLocked` (see the
   §8.2 pseudocode, step (3))** — precisely so the `TIOCGPGRP` ioctl and the app-death-edge
   synthetic teardown `model.Write` can **never** precede or delay the live fan-out. Placing
   it before `fanOutLocked` would add a syscall (and possibly a `model.Write`) of latency to
   live output, violating NG1; the ordering is therefore not incidental but **load-bearing**,
   and is asserted by `TestRegression_LivePathUnchangedRaw` (§13.3). This keeps a
   live-attached idle shell clean within ~¼ s.
2. **Unconditionally at the top of `Snapshot()` / `Serialize` paths**, under the same
   `s.mu`, **before** reading the model. This is the load-bearing site: `suspend()`'s
   `Snapshot()` serializes with **no attach**, so without sampling here a SIGKILLed app's
   stale alt/mouse modes would be baked into the `.buf` and leak on restore. Sampling
   immediately before every `Serialize` (attach **and** flush **and** suspend **and**
   shutdown) closes that hole. Cost: one extra ioctl per attach/flush/suspend — negligible.
3. **In `runMaintenanceOnce`** (existing cadence) so a session with **no** live clients
   and no output still converges to a clean idle state between flushes.

The debounce (1) keeps steady-state cost bounded; (2) makes correctness independent of
the debounce timing — the reset is **always** applied before the model is serialized,
never only at attach. The syscall-cost tradeoff is explicit: we pay a few extra cheap
ioctls per second per busy session plus one per serialize, in exchange for never leaking
stale modes and never adding a syscall to the per-chunk path.

`OnForegroundReset`:

1. Feeds the model a **synthetic, canonical, NON-DESTRUCTIVE app-teardown sequence** —
   model-only, **never sent to live clients** (NG1: the live path is untouched; a live
   client watching a killed alt-screen app sees the same residue any real terminal would,
   out of scope):
   `ESC[?1049l ESC[?47l ESC[?1047l ESC[?1l ESC[?6l ESC[?1000l ESC[?1002l ESC[?1003l ESC[?1004l ESC[?1006l ESC[?1015l ESC[?7h ESC[?25h ESC[?2004l ESC(B ESC)B SI`.
   **All three alt-screen exits (`?1049l`, `?47l`, `?1047l`) are emitted** because an app
   may have *entered* the alt buffer via the legacy `?47h` or `?1047h` (vim/less under some
   terminfo still do), and `?1049l` alone does **not** take x/vt out of a buffer entered
   via `?47h`/`?1047h` — x/vt would stay in the alt grid while `resetTransientModes()` force-
   sets `shadow.altScreen=false`, producing a contradictory payload (shadow says primary →
   step 5 emits scrollback, but x/vt's getters return the stale ALT grid for `Render`).
   Emitting all three guarantees x/vt leaves the alt buffer regardless of which DECSET
   variant entered it, keeping x/vt and the shadow consistent.
   The trailing `ESC ( B ESC ) B` re-designates **G0 and G1 to US-ASCII** and `SI`
   re-invokes G0 into GL — resetting charset/locking-shift state **without** touching the
   grid. **RIS (`ESC c`) is explicitly FORBIDDEN here:** RIS is a full reset that clears
   the grid AND scrollback, which would wipe the shell screen on every foreground-app-death
   edge — the exact opposite of this method's contract. The unit test asserts the teardown
   bytes contain no `ESC c`. Feeding the sequence *through `model.Write`* makes x/vt itself
   leave the alt buffer and clear its modes, and the callbacks update the shadow in step —
   so x/vt and shadow stay consistent.
2. As a belt-and-braces guard against a missing callback, also calls
   `shadow.resetTransientModes()` (§7) directly. Both are idempotent: if the app exited
   cleanly (its own `?1049l`/`?1000l` already ran) re-disabling already-off modes is a
   no-op, so firing on every app→shell edge is safe.

This does **not** clear the grid, scrollback, title, or cursor position — those are
legitimately still on screen at the prompt. (This is exactly why RIS is forbidden above.)
The next real app output re-establishes whatever modes that app wants, parsed normally.
The killed-alt-screen-app re-attach case is covered by
`TestRegression_ReattachAfterKilledAltScreenApp` (§13.3), and the non-destructiveness of
the teardown — that `OnForegroundReset` clears modes but **preserves grid + scrollback +
title + cursor position** — is asserted directly by a `model/` unit test
(`TestOnForegroundReset_PreservesScreen`, §13.1).

### 11.2 Force-suspend of a LIVE alt-screen app (suspend-of-alive contract)

§11.1 fires `OnForegroundReset` only on the **edge** where the foreground process group
returns to the shell. `BeginForceSuspend` (terminal.go ~666–713) is the opposite case: it
deliberately suspends a **live, non-idle** session whose app is **still running in the alt
buffer** (vim/htop). At the suspending `Snapshot()`, `checkForegroundResetLocked()` samples
`TIOCGPGRP`, sees the app **still foreground** (no app→shell edge), and therefore does
**NOT** clear `shadow.altScreen` / the modes. Without a fix, the alt grid + `?1049h` would be
baked into the `.buf`; on restore `NewRestored` spawns a **fresh shell** (the app is gone)
but would replay `?1049h` + the dead app's last alt frame, stranding the re-attaching client
in a **frozen alt buffer** with no app left to repaint or emit `?1049l`. The §13 alt tests
cover live-reattach and killed-app-reattach but never this **suspend-while-alive-then-restore**
path — so it is specified and tested here.

**Contract: force-suspend is treated as an app-death edge, and the teardown + notice +
serialize are ONE atomic critical section (Issue: the three-acquisition race).** Because
`BeginForceSuspend` is about to **kill** the foreground app, the force-suspend path
**unconditionally drives the model teardown BEFORE the suspending serialize**. The three
operations — `OnForegroundReset` (model→primary), the notice injection, and the serialize —
must be performed in **a SINGLE uninterrupted `s.mu` hold**, *not* as three separate
acquisitions.

**Why the naive three-call form is wrong.** `s.model.OnForegroundReset()` (a model call),
`s.InjectLocal(notice)` (takes `s.mu` internally), and `s.Snapshot()` (takes `s.mu`
internally) are **three separate `s.mu` acquisitions**. At this point `BeginForceSuspend`
has only set `suspending = true`; **the PTY is still live and the pump goroutine is still
reading**. A live alt-screen app (htop refresh, vim redraw) can deliver a `pumpStep` chunk
in the gap *between* two of those acquisitions. That chunk repaints alt content onto the
model the teardown just forced to primary (a periodic frame does **not** re-emit `?1049h`),
or re-enters alt — so the subsequent serialize captures a corrupted/again-alt buffer and
persists it, defeating the very contract this section adds. A test cannot deterministically
catch that interleave, either.

**Mechanism (chosen): a dedicated combined method that fuses all three under one hold.**

```go
// forceSuspendSnapshotLocked performs the entire §11.2 teardown AND the suspending
// serialize as ONE uninterrupted s.mu critical section, so NO pumpStep chunk (the PTY is
// still live until BeginForceSuspend kills it) can interleave between the teardown and the
// serialize and repaint alt content onto the model we just forced to primary. It is the
// SOLE place these three operations are fused, and the ONLY model teardown applied without
// a real pgrp edge — justified because force-suspend itself IS the app's death.
// Caller (BeginForceSuspend) must NOT already hold s.mu. Returns the blob to persist.
func (s *Session) forceSuspendSnapshotLocked(notice []byte) (blob []byte) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.mutateModelLocked(s.model.OnForegroundReset) // (1) alt-exit + mode reset, model-only (§11.1), §8.5-recovered
    s.injectLocalLocked(notice)                 // (2) notice into PRIMARY, model-only, no fan-out
    blob = s.header() + s.serializeLocked() // (3) clean primary captured atomically, §8.5-recovered
    s.dirty, s.lastBlob = false, blob
    return blob
}
```

- **`injectLocalLocked`** is the lock-free core of `InjectLocal` (§12): `InjectLocal` becomes
  a thin `s.mu.Lock(); defer Unlock(); s.injectLocalLocked(b)` wrapper, and
  `forceSuspendSnapshotLocked` calls the core directly so it does **not** re-acquire `s.mu`.
  The primary-forcing defensive guard (§12) lives in `injectLocalLocked`, so the notice still
  cannot land in an alt buffer even if step (1) were somehow skipped.
- **`BeginForceSuspend` persists the RETURNED blob verbatim and does NOT re-`Snapshot()`.**
  Because the blob is captured *inside* the single hold, the only thing that could change the
  model afterward — a live `pumpStep` chunk arriving between the hold's release and the PTY
  kill — **cannot affect the persisted bytes** (we already hold the clean-primary blob; we do
  not re-serialize). Re-calling `Snapshot()` after the kill is explicitly forbidden here
  precisely because it could re-serialize a model a late chunk re-alt'd.
- **Live clients are unaffected.** The teardown sequence is fed **through the model only**
  (`OnForegroundReset` / `injectLocalLocked` never call `fanOutLocked`); any live client
  watching the about-to-be-killed app sees the same residue a real terminal would (NG1).

**The shared `suspend()` is restructured to branch on `force` (minor Issue: the legacy
`append([crowbar]…)` notice + single shared `Snapshot()` must not coexist with the fused
path).** Today `terminal.go`'s `suspend(ctx, sid, force)` is a single implementation that does
`scrollback := s.Snapshot()` then `if force && wasRunning { scrollback = append(scrollback,
[crowbar]…) }`, all under one `flushMu` hold. Under this design that raw byte-append is
**deleted** (it would corrupt the §12 blob and break the §13.3 fixpoint) and the two cases
diverge. The restructured form, with the `flushMu → s.mu` ordering of §8.4 preserved:

```go
// suspend() persists the session state then swaps in a placeholder and kills the PTY.
// flushMu wraps the WHOLE capture+write of BOTH branches (serialises with cadence-flush,
// detach, and shutdown per §8.4); s.mu is taken INSIDE the snapshot call, never around it.
func (e *terminalEngine) suspend(ctx context.Context, sid string, force bool) error {
    // ... unchanged: lockSession, registry Get/IsLive, Begin{Force}Suspend, workspace lookup ...
    fm := s.FlushMu()
    fm.Lock()
    var blob []byte
    if force && wasRunning {
        // Force path: ONE fused critical section (teardown + notice + serialize) inside
        // forceSuspendSnapshotLocked, which takes s.mu itself. NO second Snapshot(), NO raw
        // byte-append — the notice is injected via injectLocalLocked into the PRIMARY model
        // and captured atomically (§11.2). Persist the returned blob VERBATIM.
        blob = s.forceSuspendSnapshotLocked(
            []byte("\r\n[crowbar] session suspended to free resources; re-open to restore\r\n"))
    } else {
        // Non-force (and force-but-idle) path: a plain serialize. Snapshot() takes s.mu
        // internally, samples the §11.1 foreground reset, and returns (blob, changed); the
        // suspend path ignores changed and always writes. No notice, no teardown fusion.
        blob, _ = s.Snapshot()
    }
    if dir != "" {
        if err := persistence.WriteBuf(dir, sid, blob); err != nil { /* log */ }
    }
    fm.Unlock()
    // ... unchanged: saveMeta, placeholder swap, kill ...
}
```

Concretely: **(1)** the legacy `scrollback = append(scrollback, []byte("…[crowbar]…")…)` block
is **removed** — the notice now enters the *model* via `injectLocalLocked` (§12), never the
persisted bytes, so it can never be double-applied; **(2)** the force-and-running branch calls
`forceSuspendSnapshotLocked(notice)` exactly **once** and `WriteBuf`s the returned blob with no
re-`Snapshot()` (re-serializing after the fused hold is explicitly forbidden, §11.2); **(3)** the
non-force branch (and the rare force-but-already-idle case, where there is no live app to tear
down) calls `s.Snapshot()` and writes its blob; **(4)** both branches run their capture+write
under the **same outer `flushMu` hold** (§8.4), while `s.mu` is acquired *inside*
`forceSuspendSnapshotLocked` / `Snapshot()` — never wrapped around them — so the `flushMu → s.mu`
order is identical on both paths and the §8.4 deadlock-freedom analysis is unchanged. This makes
the call-site shape unambiguous and matches the exhaustive enumeration the rest of the spec uses.

After step (1) x/vt is out of the alt buffer, so its getters return the **restored primary**
grid (the pre-app shell screen x/vt saved on `?1049h`); step (2)'s `[crowbar] …` notice lands
in that primary; step (3) serializes a **clean primary** screen + scrollback (step 5 present,
no `?1049h`), never a frozen alt grid. On restore the session deterministically comes up in
the primary buffer showing the shell screen and the prompt renders normally. The dead app's
last alt frame is intentionally dropped (the app is being killed). The interleave is closed by
construction and asserted by `TestRegression_ForceSuspendInterleaveRace` (§13.3), which fires a
concurrent alt-repaint chunk during the force-suspend and asserts the persisted blob is still
primary regardless.
`TestRegression_ForceSuspendAltScreenThenRestore` (§13.3) drives an app into alt-screen +
mouse, force-suspends it while alive, restores, and asserts the restored payload is the
**primary** (no `?1049h`/`?47h`/`?1047h`/mouse, scrollback present) and the screen is clean.

---

## 12. Persistence

- `.buf` files now store **serialized model state + a size header**, not raw PTY history.
  `WriteBuf`/`ReadBuf`/`DeleteBuf` stay byte-blob APIs and **need no change**; the header
  lives *inside* the blob. **Why the header is mandatory:** the §6 serialized bytes encode
  *one* grid width (autowrap and absolute moves are width-specific, and stored scrollback
  is un-reflowed — §10(a)). Restoring those bytes into a model built at a *different* size
  would wrap the grid wrong and corrupt the screen. So the size travels with the bytes.
- **Blob format.** `Snapshot()` returns a small fixed self-describing header followed by
  the §6 redraw bytes:

  ```
  CRWB1 <cols> <rows> <alt 0|1> <scrollbackLines>\n
  <serialized §6 redraw bytes …>
  ```

  `CRWB1` is a version tag (lets us reject/skip non-matching blobs without migration code).
  `<alt>` is the alt-screen flag at snapshot time — redundant with the `?1049h` already in
  the bytes, but it lets restore size and branch deterministically without parsing the
  payload. The header is a single ASCII line; the redraw bytes follow verbatim.
- **`Session.header()` — the single header builder, sourced entirely from
  `TerminalModel.HeaderState()` (Issue: all four fields must be constructible with zero
  further decisions).** The header is a *Session* method (called verbatim in the §8.4
  `Snapshot()` and §11.2 `forceSuspendSnapshotLocked` blobs as `s.header() + …Serialize(…)`),
  but two of its four fields — `<alt>` and `<scrollbackLines>` — have no source on the
  session side except through the model. Rather than expose `IsAltScreen()` and thread
  `scrollbackLines` onto the `Session`, the header reads **all four** fields from the one
  narrow `HeaderState()` accessor (§4), so the header never re-parses the stream and the
  blob line is buildable verbatim:

  ```go
  // header builds the mandatory CRWB1 size line. It is the SOLE constructor of that line and
  // reads every field from the model's HeaderState() (§4) under the s.mu the caller already
  // holds. Called only by the live-session blob paths (Snapshot §8.4, forceSuspendSnapshotLocked
  // §11.2); a model-less placeholder never calls it — it returns its stored s.rawBlob, whose
  // header was written when it was last a live session (§4.2/§8.4).
  func (s *Session) header() string {
      cols, rows, alt, sb := s.model.HeaderState()
      altBit := 0
      if alt { altBit = 1 }
      return fmt.Sprintf("CRWB1 %d %d %d %d\n", cols, rows, altBit, sb)
  }
  ```

  Because `HeaderState()`'s `alt` is `emu.IsAltScreen()` (the §4.2 single-source-of-truth
  value Serialize also uses for steps 3/5/7), the header `<alt>` and the body's `?1049h` can
  never disagree. `scrollbackLines` comes from the compact ring's cap (`scrollback.Cap()`,
  §9.2.2), the same value `model.New(cols, rows, scrollbackLines)` was built with — so a
  restore rebuilds the model at the identical depth.
- The session exposes this via the **same `Session.Snapshot()` method** the persistence
  paths already call (`flushSessionOnce`, `persistOnDetach`, `Suspend`, `Shutdown`), now
  returning **`(blob []byte, changed bool)`** (§8.4). For a **live** session `Snapshot()`
  returns `header + serializer.Serialize(s.model)` under `s.mu` (after the §11.1
  foreground-reset sample), instead of `ring.Snapshot()`, with `changed` = the dirty bit
  consumed in that same hold. For a **placeholder** `Snapshot()` returns the
  **already-persisted blob verbatim** with `changed == false` — it holds no model, so there
  is nothing to re-serialize (§4.2). The `flushMu → s.mu` ordering this introduces is
  analysed deadlock-free in §8.4. **The terminal-event persist call sites
  (`persistOnDetach`, `Suspend`, `Shutdown`) ignore `changed` and always write; only the
  10 s cadence `flushSessionOnce` skips the disk write when `!changed`** (§8.4).
- **Restore builds a live model; the header is the single size source.** `NewRestored`
  (a real PTY) takes the **raw blob**, and the shared `spawn()` helper is the **one place**
  that **parses the `CRWB1` header, splits off the redraw**, resolves `cols × rows` and
  `scrollbackLines` from the header (never from FE dims — §4.2/§6 Issue: the redraw is
  width-specific and un-reflowed, so header size MUST win), constructs a fresh model at the
  header's size, **calls `pty.Setsize(headerCols, headerRows)` right after `pty.Start` and
  before the first `Read`** (so the restored PTY is born at the persisted size, not the
  kernel 80×24 default — §4.2), and feeds `model.Write(redraw)` before the pump starts — so
  the restored model reproduces the saved screen at the *correct* width and the shell's
  first output is parsed at that same width. If the new client later attaches
  at a different size, the **first client resize** reshapes both PTY and model in lockstep
  (§4.2). (The old "prepend scrollback to ring so it replays first" trick is gone; the model
  simply *is* the saved state.) A malformed/absent header (or a `CRWB1`-mismatch) is treated
  as an empty session — safe, since pre-production we clear old state anyway.
- **Placeholders parse the header for size/accounting but build NO model.**
  `NewPlaceholder` / `LoadPlaceholder` retain the **raw blob** and may read the cheap
  `CRWB1` header line for size metadata, but they do **not** construct a `vtModel` and do
  **not** `model.Write(redraw)` (§4.2): a placeholder has no PTY and is never serialized
  for attach. `ModelBytes()` returns `len(blob)`. When the engine attaches a placeholder it
  re-reads the `.buf` and spawns a live `NewRestored`, which is where the model is finally
  built. This is the deliberate avoidance of pinning a full model per suspended session.
- **User-visible notices (`restoreCWD`, force-suspend) are injected via a concrete
  model-only API AFTER model construction — never baked into the persisted blob.** Today
  `restoreCWD` runs *before* `spawn()` and returns mutated scrollback bytes, and the
  force-suspend notice appends raw `[crowbar] …` text onto the scrollback bytes. Under the
  blob format that would corrupt the redraw and break the §13.3 fixpoint. The injection path
  is made concrete, not hand-waved:

  ```go
  // InjectLocal feeds a clean-ANSI chunk into THIS session's model only. It NEVER fans the
  // bytes out to any client (no fanOutLocked call). It is the sole sanctioned way for the
  // engine to push synthetic, daemon-authored on-screen text (restore/suspend notices) into
  // the model so it surfaces on the NEXT Serialize without ever touching the persisted .buf
  // or the live wire. Distinct from pumpStep, which fans out raw PTY bytes; InjectLocal is
  // engine-authored and model-only.
  //
  // It is a thin wrapper that takes s.mu and delegates to injectLocalLocked; the lock-free
  // core exists so §11.2's forceSuspendSnapshotLocked can fuse the notice injection with the
  // teardown + serialize in ONE s.mu hold (calling the core directly, not re-acquiring s.mu).
  func (s *Session) InjectLocal(b []byte) { s.mu.Lock(); defer s.mu.Unlock(); s.injectLocalLocked(b) }

  // injectLocalLocked is the lock-free core: caller holds s.mu. It feeds bytes to the model
  // via writeModelLocked (the §8.2/§8.5 Write-path recover — NOT a bare model.Write), so a
  // parse panic on a daemon-authored notice bumps modelPanics and returns instead of escaping
  // to the restore-spawn or force-suspend goroutine (§8.5 enumerated recover set). It sets
  // s.dirty + clears s.lastBlob, but never fans out.
  //
  // PRIMARY-BUFFER CONTRACT (Issue): a daemon notice must surface in the user-visible
  // PRIMARY screen/scrollback, never in a transient alt buffer (where it would be lost on
  // the app's next ?1049l and never reach primary history). Both sanctioned callers already
  // guarantee primary: restore spawns a FRESH shell (primary), and force-suspend runs
  // OnForegroundReset → primary BEFORE injecting, all in one hold (§11.2). As a hard,
  // defensive guard, injectLocalLocked — if shadow.altScreen is somehow still set — FIRST
  // drives the model to primary by feeding the canonical alt-exit
  // `ESC[?1049l ESC[?47l ESC[?1047l` through writeModelLocked, THEN writes b (also via
  // writeModelLocked). So an injected notice can NEVER land in the alt buffer, and neither the
  // alt-exit nor the notice write can let an emulator panic escape (§8.5).
  func (s *Session) injectLocalLocked(b []byte)
  ```

  - **`restoreCWD` is SPLIT, not deleted.** Today `restoreCWD` does **two** things: (1) it
    **selects** the working directory for the respawned shell — `dirExists(savedCWD)` and,
    when the saved cwd no longer exists, falls back to the user's **home dir** — and (2) it
    appends a user-visible `[crowbar] …` notice onto the scrollback bytes. These are now
    cleanly separated:
    - **(a) cwd RESOLUTION stays PRE-`spawn()` and is unchanged.** The `dirExists → home
      fallback` selection MUST run before `spawn()`, because `spawn()` sets `cmd.Dir` and the
      PTY must be started in a valid directory — spawning into a deleted cwd fails and strands
      the placeholder. So a `resolveRestoreCWD(saved string) string` helper (the kept half of
      the old `restoreCWD`) returns **only the resolved cwd** (saved if it exists, else home),
      and `spawn()` uses it for `cmd.Dir`. The home-fallback behavior survives byte-for-byte;
      only its return type changes (a path string, no scrollback mutation).
    - **(b) the NOTICE moves to a model-only `InjectLocal` carried on `engineBirth.Notice`,
      injected BEFORE the session is registered (Issue: the post-`spawn` injection raced
      `reg.Add`).** The scrollback-byte mutation is the **only** part removed. The
      `[crowbar] restored …` text is passed into the **engine-level** `spawn` via
      `engineBirth.Notice` (§9.1), and `e.spawn` calls `s.InjectLocal(notice)` **between
      `session.NewRestored` and `e.reg.Add`** — i.e. while the session is **not yet in the
      registry and therefore not yet attachable**. The session-level `spawn()` keeps **no**
      notice parameter (minimal, §9.1). This closes the race the earlier "`restore()` calls
      `InjectLocal` after `spawn()` returns" wording could not: `e.reg.Add` (inside the
      engine-level `spawn`) is exactly what makes a session attachable, so any injection *after*
      that returns could be beaten by a concurrent `Attach` serializing between `reg.Add` and the
      notice. Injecting before `reg.Add` makes "the notice is on the model before the first attach
      can observe the session" an **achievable invariant**, not a hope. The notice still never
      touches the persisted blob (preserving the §13.3 fixpoint) yet appears on screen for the
      first re-attaching client.
  - **Force-suspend notice** is injected via `injectLocalLocked` **inside**
    `forceSuspendSnapshotLocked` (§11.2) — fused with the teardown and the suspending
    serialize in ONE `s.mu` hold, instead of appending to bytes — and **after** the teardown
    drops the model to the **primary** buffer, so the notice lands in the user-visible primary
    screen, not the alt buffer of the app being killed. The single-hold fusion is what
    prevents a live `pumpStep` chunk from interleaving between teardown and serialize (the
    §11.2 Issue). (Even absent the ordering, injectLocalLocked's defensive primary-forcing
    guard above would relocate it to primary.)

  Because the notice enters through `InjectLocal` → `model.Write` (the live parse path, not
  the persisted bytes), the `.buf` it was restored from stays a clean fixpoint; only the
  *next* freshly-serialized blob contains the notice as ordinary on-screen text (it lands
  after the restored screen and scrolls naturally). `TestRegression_SerializeFixpointThroughXVT`
  (§13.3) asserts blob byte-identity on the **clean** (no-`InjectLocal`) restore path AND
  asserts that an `InjectLocal`'d session's source `.buf` is **unchanged** (notice-free)
  while its screen shows the notice; sessions that received a notice are asserted for
  **visible-buffer** equivalence (the notice is legitimately new screen content), not blob
  byte-identity.
- **No migration.** Pre-production, no users. Old raw-byte `.buf` files are not converted.
  Dev IndexedDB / on-disk `.buf` are cleared as part of rollout (consistent with the
  "no legacy migration" project rule). A stale raw `.buf` fed into the model would simply be
  parsed as PTY bytes — harmless, but we clear them anyway for cleanliness.

---

## 13. Test & coverage plan

### 13.1 Unit tests (target: 100% coverage of `model/`)

- **`shadowState`:** every callback path (Title, IconName, AltScreen, CursorVisibility,
  CursorStyle, EnableMode/DisableMode for each tracked + each ignored mode, ScrollRegion,
  WorkingDirectory); `setMode` accept/ignore branches; `newShadowState` defaults.
- **`vtModel`:** `Write` advances grid; `Resize` reshapes grid and does **not** reflow
  scrollback; `Title/Cols/Rows`; scrollback eviction → compact line stored **with its
  `wrapped` flag**; `Close` idempotent; `ModelBytes` math (live model `m.ModelBytes() +
  len(s.lastBlob)` vs placeholder `len(s.rawBlob)` with the `s.model == nil` fast-path).
- **`Write` panic recovery (§4.1/§8.2 — `TestModelWritePanic_RecoversToParseableState`):**
  using the **`newEmulator` seam (§4.1)** — the test reassigns the package-level
  `newEmulator` var to return an `emulator` impl whose `Write` panics on a sentinel byte,
  with `defer`-restore (no build tag, file-local) — feed a panic-inducing chunk and assert
  the adapter **recreates the
  emulator to a known-blank, parseable state**: `degraded` is set, `parsePanics == 1`, the
  compact scrollback ring + `sbBytes` are **preserved** (history not lost), the grid is blank,
  and — the correctness assertion the Issue requires — **subsequent normal `Write`s parse
  correctly** and a `Serialize` → fresh `vtModel` → re-`Serialize` round-trip reproduces the
  post-recovery screen byte-identically (proving the parser is NOT left corrupted / "one chunk
  behind"). A second panic re-recovers idempotently. The session-level backstop is covered
  separately by asserting `Attach` still serves after a panic.
- **`trackPendingPartial` / `PendingInput` (§4.1/§8.3 — mid-sequence attach re-sync):**
  table-driven over the escape-framing tracker — a chunk ending in ground state yields
  `PendingInput() == ""`; a chunk ending mid-CSI (`ESC[`), mid-OSC (`ESC]2;ti`), mid-DCS, and
  mid-`ESC`-intermediate each yield the **exact in-flight bytes**; the partial is **carried
  across** a subsequent `Write` and **cleared** the moment the tail completes the sequence; a
  `CAN`/`SUB` mid-sequence resets to ground (`PendingInput() == ""`); 8-bit C1 introducers
  (0x9B/0x9D) are tracked; an in-flight sequence longer than `maxPendingPartial` yields `""`; and
  a recovered parse panic (§8.2) clears `pendingPartial`. Plus an integration-style case: after a
  mid-CSI `Write`, `Attach`'s payload equals `Serialize(model) + PendingInput()` while
  `Snapshot()`'s blob equals `header + Serialize(model)` with **no** partial appended.
- **Per-row wrap flag sourced live from x/vt (§9.2 — wrap-flag-for-evicted-rows):** using the
  injectable `emulator` seam (§4.1) whose **eviction hook** delivers scripted `(cells, wrapped)`
  per evicted row and whose `RowWrapped` returns a scripted per-visible-row signal, write a long
  logical line that soft-wraps across ≥2 physical rows, scroll it off the top, and assert each
  evicted compact line carries the **`wrapped` flag the hook delivered for that exact row at
  eviction time** (the wrapped rows `true`, the hard-break tail `false`) — proving the capture
  reads the flag **per evicted row from the backend hook**, not from any adapter-maintained side
  array (there is none) and not from the cursor's current row. A hard-break line evicts with
  all-`false` flags. A second case is the **per-`Write` ceiling guard (blocker resolution):** with
  the seam's hook scripting a fixed wrap pattern, a single multi-screen 64 KB `Write` that evicts
  **far more than the old 256-line window** in one call stamps **every** evicted row with exactly
  the hook's reported flag and the ring's line-count equals the **full** eviction count — proving
  the hook has no per-`Write` capture ceiling (the deleted diff fallback would have captured only
  the last ~256) and the flag never depends on chunk-boundary cursor deltas.
- **`Resize` eviction (§4.1/§10(a)):** a height-**shrink** that overflows the grid drains the
  displaced top rows into the compact ring — assert the ring's line-count grows by exactly the
  overflow and `sbBytes` increases by the encoded byte sum; a subsequent height-**grow**
  neither un-drains nor double-counts (ring count + `sbBytes` unchanged by the grow), and adds
  blank rows at the bottom (no pull-back); zero-delta resize is a no-op.
- **`Resize` during alt-screen (§9.2 "Alt-screen drain guard" — blocker Issue):** enter
  alt-screen (`?1049h`), fill the alt grid, then height-**shrink**; assert the **primary**
  compact ring line-count and `sbBytes` are **unchanged** (the displaced alt rows did NOT enter
  the primary ring), and that after `?1049l` + serialize the primary buffer is the clean
  pre-alt screen (no alt rows leaked into scrollback). A paired case: a full-screen scroll
  *while in alt* likewise leaves the primary ring untouched.
- **`Resize` width-change wrap flag stays live (§4.1 `Resize` / §9.2 — major Issue):**
  write a soft-wrapped logical line across ≥2 rows at width `W`, **resize the width** to `W'`,
  then scroll the affected rows off the top; with the seam's eviction hook scripted to
  deliver the **post-resize** wrap state, assert each evicted compact line's `wrapped` flag
  reflects the **new** width — never a stale `W`-width flag — purely because the flag comes from
  the backend hook at eviction time (there is no side array to recompute or reset). A height-only resize is likewise
  driven through the seam. Covers width-shrink and width-grow.
- **Partial vs full-screen scroll-region drain (§9.2 "Partial-region drain guard" — major
  Issue):** with an active **partial** DECSTBM region (`scrollTop>1 || scrollBottom<rows`),
  drive scrolls inside the region and assert **no** lines enter the compact ring (`sbBytes`
  unchanged); then set a **full-screen** region (`1..rows`) and assert scrolls **do** drain
  (count + `sbBytes` grow), each evicted line carrying the seam's reported wrap flag.
- **`encodeRow` (§9.2.1):** a non-wrapped row trims trailing default blanks; a wrapped row is
  full `cols` width (no trim); SGR emitted only on pen change in canonical order; row ends in
  ground state; **byte-identity** — `encodeRow` of a given row equals the grid `Render()`'s
  bytes for the same row (the fixpoint primitive, also gated at P0 §15 item 6).
- **`encodeRow` wide glyph at right margin (`TestEncodeRow_WideGlyphRightMargin`, §9.2.1 —
  minor Issue):** build a row whose penultimate column holds a printable cell and whose final
  column is the **blank spacer** left by a double-width CJK glyph that did not fit (the glyph
  wrapping to the next row). Assert encodeRow treats the row as `wrapped`, encodes it at **full
  `cols` width with the spacer preserved** (trimming stops at the spacer, never swallowing it),
  and is **byte-identical** to the grid `Render()` for that row — so the deferred wide glyph
  re-wraps at the identical column on restore. A control case: a non-wrapped row with genuine
  trailing default blanks beyond the last glyph still trims those blanks.
- **`OnForegroundReset` (`TestOnForegroundReset_PreservesScreen`):** after entering
  alt-screen + enabling mouse/bracketed-paste/app-cursor-keys/non-default charset+SO and
  writing visible content, title, and a moved cursor, `OnForegroundReset` clears **all**
  those modes (alt off, mouse/focus/paste off, autowrap back ON, G0/G1='B', `glLock=0`)
  **and leaves grid cells, scrollback, title, and cursor position untouched**. Asserts the
  synthetic teardown bytes contain **no `ESC c` (RIS)** and **do contain `?1049l`, `?47l`,
  and `?1047l`**. **Run as a table over all three alt-screen entry variants** — `?1049h`,
  `?47h`, and `?1047h` — asserting that after `OnForegroundReset` **x/vt's `IsAltScreen()`
  returns false** (not just `shadow.altScreen`) for **each**, so x/vt and the shadow can
  never disagree on which buffer is active. Idempotent on a second call.
- **`Serializer.Serialize`:** assert the **exact byte order** of §6 for representative
  states (empty screen; colored text; cursor not at home; hidden cursor; bar cursor shape
  *explicitly set* vs *never set*; active mouse+bracketed-paste modes; **autowrap disabled**
  `?7l`; **origin mode `?6` active WITH a non-full-screen scroll region and the cursor off
  region-home** (asserts `?6h` precedes the CUP and the CUP is region-relative so the decoded
  cursor lands on the real cell); **non-default G0/G1 charset**; **active locking-shift `SO`
  (GL=G1) vs default `SI` (GL=G0)**; **default fg/bg/cursor color explicitly set vs never
  set**; scroll region set; alt-screen active **(scrollback step omitted)**; non-empty
  scrollback **(primary only, including a soft-wrapped logical line spanning ≥2 stored rows —
  asserts no `\r\n` between the wrapped row and its continuation, full-width wrapped row, and
  `\r\n` after the non-wrapped tail)**; icon-only / title-only / both / neither). Table-driven,
  golden-byte assertions. Specific gated-step assertions: step 2 emits `OSC 10/11/12`
  **iff** the matching `*Set` flag is true and is **omitted entirely** when no color was
  app-set (so the client theme is never overridden), **and the payload is the canonical
  `rgb:RRRR/GGGG/BBBB` 16-bit lowercase-hex form (§6)** — a dedicated case asserts the exact
  bytes (e.g. `ESC ] 11 ; rgb:ffff/ffff/ffff ST`, `ST`-terminated, never bare BEL) and that
  feeding our own emitted OSC color back into a fresh `vtModel` and re-serializing is
  **byte-identical** (the color self-fixpoint through x/vt); step 1 always emits an explicit `?7h`
  after DECSTR (deterministic autowrap-ON baseline); step 9 emits `?7l` **iff**
  `modes[7]==false` and omits it otherwise; step 10 emits `SO` **iff** `glLock==1`; step 14
  emits DECSCUSR **iff** `cursorShapeSet`; **step 12 `?6h` lands BEFORE step 13 CUP (never
  after — the origin-mode ordering fix), and when origin mode is ON the step-13 CUP is
  emitted in region-relative coordinates (`row = absRow − (scrollTop − 1)`), asserted by an
  origin-mode-active golden case whose decoded cursor lands on the real cell**; step
  11 DECSTBM lands **after** the grid paint; scrollback step 5 emits **no separator after a
  `wrapped` row and `\r\n` after a non-wrapped row**; step 5 is **absent** when `altScreen`.
  Determinism: same model → identical bytes across runs (modes ascending, lines
  chronological, OSC selection per §6).
  **These golden-byte tests are `x/vt`-adapter-specific** — they pin *our* exact encoder
  output and are **not** part of the cross-backend gate. The `midterm` adapter gets its own
  golden file; the only cross-backend contract is the §13.2 buffer-equivalence harness
  (see §5.4).
- **`sanitizeOSCText` (§6 step 16):** table-driven — plain text passes unchanged; embedded
  `ESC` / `BEL` / arbitrary C0 (0x00–0x1F) and `DEL` (0x7F) are **dropped**; C1 controls
  (incl. 0x9C 8-bit ST) dropped; invalid UTF-8 → U+FFFD; over-long input truncated at
  `maxOSCTextRunes` on a rune boundary; an all-control input → `""` (so the OSC is omitted).
  Plus a serializer-level case: a `shadow.title` containing `"\x1b[31mred\x07"` serializes to
  a **single** well-formed `ESC ] 2 ; red ST` with no embedded ESC/BEL and a 7-bit `ST`
  terminator.
- **Negative assertions in unit form:** `Serialize` output contains **no** `ESC [ ... c`
  (DA), **no** `ESC [ ... n` (DSR), **no** `ESC [ ... R` (cursor reply), exactly **≤1**
  OSC title, **no `ESC` or `BEL` inside any OSC title/icon parameter** (the sanitization
  guarantee), and ends with `ESC [ 0 m` and in ground state (parser-state check).

### 13.2 Conformance harness vs `xterm.js` `SerializeAddon`

**Oracle bridge — pinned, not an either/or.** The harness is a Go test (build-tagged
`conformance`) that **shells to a small Node subprocess** running a headless `xterm.js` +
`SerializeAddon` script over stdin/stdout JSON. We pick the **Node subprocess** over jsdom
because the jsdom path still needs Node *and* drags in a DOM shim that `SerializeAddon`'s
buffer reads do not require — the subprocess is the smaller, more faithful surface. CI
provisioning: the harness is **skipped with a clear `t.Skip` when `node` is absent**
(`exec.LookPath("node")`), and CI installs Node (the `web/` workspace already requires it,
so the toolchain is present); the conformance job is a required gate on PRs that touch
`model/` or bump the pins, so it cannot be silently skipped there.

**Pinned oracle version.** The exact `xterm` (`@xterm/xterm`) and `@xterm/addon-serialize`
versions are pinned in a dedicated `model/conformance/package.json` with a committed
lockfile, and recorded in `model/UPSTREAM.md` **alongside the `x/vt`/`ultraviolet` commit
pins (§5.3)** so the conformance bar cannot silently move with an `xterm.js` release. Bumping
the oracle version is the same gated event as bumping the x/vt pin: accepted only after the
harness re-passes at the new version.

For each fixture the harness:

1. feeds the fixture's raw PTY capture into **both** our model **and** the headless
   `xterm` + `SerializeAddon`;
2. serializes both;
3. feeds **each** serialization into a **third** fresh `xterm` and compares the resulting
   **buffer** for equivalence, scoped to **exactly the state `xterm.js` can expose** — the
   public `buffer.active` API: every cell's glyph + combining marks + width, its SGR
   (fg/bg/bold/italic/underline/inverse/etc.), `cursorX`/`cursorY`, **plus the scrollback
   physical-line-count, each line's `isWrapped` wrap state, and the newest scrolled-off line
   (Issue/§G3 + soft-wrap fidelity)**. This is the backend-agnostic cross-backend gate.
   **The third (reconstruction) `xterm` AND the oracle `xterm` are both constructed with
   `scrollback` ≥ the fixture/model depth (pinned in `model/conformance` to the model's
   `scrollbackLines`, default 10000, Issue): a reconstruction xterm shorter than the model's
   depth would truncate the rebuilt history and make the line-count / newest-line assertions
   fail spuriously or mask a real divergence, so the capacity is pinned, never defaulted.**
   **Trailing-blank-grid reconciliation (Issue): `SerializeAddon` TRIMS trailing blank grid
   rows below the cursor, whereas our serializer emits exactly `rows` physical rows (§6 step
   7), so on a fixture with scrollback AND trailing blank grid rows the two reconstructions
   would place the viewport-vs-history split differently and the cross-backend line-count
   diff would spuriously fail.** Therefore the **cross-backend** scrollback line-count /
   newest-scrolled-off-line assertion (against `SerializeAddon`) is made on fixtures whose
   **visible grid is full** (cursor at/near the bottom — `vim`/`htop`/`less`/`softwrap`
   naturally fill it), where both backends agree; the **trailing-blank-grid-with-scrollback**
   case — where our exact-`rows` emission is the *more* faithful behavior (the model's
   `ScrollbackLen()` is authoritative) — is gated against **our own model** in §13.3
   `RoundTripScrollbackDepth`, not cross-backend against the trimming oracle. This mirrors the
   existing split below (private modes gated against our own model, §13.2 step 4, not the
   oracle), and keeps the cross-backend gate exact and stable;
4. **Private modes are NOT read back from `xterm.js`** (it exposes no public API for
   `?1004`/`?2004`/mouse `?1000-1003`/`?1006`/`?1015`/`?7`/`?6`/`?1049` state — these live in
   its private `CoreService`/`InputHandler` and are deliberately not part of the
   cross-backend diff). Instead the mode half of buffer-equivalence is asserted against
   **our own model**: feed our serialization into a **fresh `vtModel`** and assert its
   `shadow.modes` / `IsAltScreen()` / cursor shape+visibility exactly equal the source
   model's. So grid+SGR+cursor-position+scrollback are gated cross-backend against the
   `xterm.js` oracle, while the private-mode/cursor-shape state is gated by our own
   serialize→re-parse round-trip (the §13.3 `SerializeFixpointThroughXVT` shape);
5. asserts the title separately (our deviation, §6.1).

**Fixtures** (real captured PTY streams): `vim`, `htop`, `tmux`, `less`, a **CJK**
wide-glyph screen, an **emoji-ZWJ** sequence screen, a **bracketed-paste** session, and
the corners the §6/§6.2 decisions newly depend on:

- **`linedraw`** — an app mid **DEC line-drawing** (G0/G1 SCS **invoked via `SO`**, e.g. a
  TUI box) so step 10 charset **and locking-shift** restoration is asserted: the harness
  writes additional line-drawing bytes *after* re-attach and asserts they render as box
  glyphs (GL=G1), proving the active `SO` invocation — not just the G1 designation — was
  restored.
- **`widewrap`** — a screen with the cursor parked in the **pending-wrap** latch at the
  right margin (and a wide CJK glyph straddling the margin). The harness writes one
  printable char *after* re-attach and asserts the result matches the pre-attach terminal
  when x/vt exposes the latch (step 13 re-arm), or is off by **at most one cell** when it
  does not (the §6.2 bounded residual) — making the actual behavior an asserted fact.
- **`oscolor`** — an app that set **default fg/bg/cursor color** via OSC 10/11/12, so step
  2 is asserted (default-attribute cells and the cursor restore to the right colors). A
  paired **`oscolor-none`** fixture (an app that never issued OSC 10/11/12) asserts step 2
  emits **nothing**, so the serializer never overrides the client theme on attach.
- **`hyperlink`** — an app that wrote **OSC 8** hyperlinked cells, pinning the actual
  `Render()` behavior (survives vs accepted-un-restored per §6.2) as a non-regressing fact.
- **`autowrap-off`** — an app that issued `?7l`, asserting the step-9 inverted-polarity
  rule (the grid still paints with autowrap ON, then `?7l` is re-asserted).
- **`softwrap`** — a build-log / `less` capture with **long logical lines that soft-wrap**
  across multiple physical rows in BOTH the visible grid and the stored scrollback. Asserts
  the step-5/step-7 wrap handling against the oracle: the reconstructed buffer's **scrollback
  physical-line-count, wrap state (`isWrapped` per row), and newest scrolled-off line** equal
  `SerializeAddon`'s — i.e. a wrapped row and its continuation are emitted with **no `\r\n`
  separator** (the wrapped row at full `cols` width) so they are NOT flattened into hard
  breaks. This is the explicit gate for the soft-wrap fidelity fix (§6 steps 5/7, §9.2).

These exercise wide cells, combining marks, alt-screen, scroll regions, mode toggles,
charset, default colors, hyperlinks, and autowrap polarity — the exact corners where
serialize fidelity breaks. The harness is the **gate** for any `x/vt`/`ultraviolet` pin
bump (§5.3) and for the `midterm` swap (§5.4). **Cross-backend gate scope:** the harness
asserts **buffer-equivalence only** (grid + cursor + modes + colors), which is
backend-agnostic; it does **not** assert byte-identity, since `midterm`'s encoder emits
different-but-equivalent bytes (see §5.4, §13.1).

### 13.3 Black-box `TestRegression_*` (in `api/tests`, integration tag)

Per repo convention (every backend bug → `TestRegression_*`, the suite is the v0 contract):

- **`TestRegression_ReattachNoQueryReplies`** — drive a session through DA/DSR/OSC-color
  queries, detach, re-attach; assert the attach payload contains **no** DA/DSR/OSC-color
  **reply** bytes and **no raw historical OSC** re-emission.
- **`TestRegression_ReattachNoRawOSCTitleReplay`** — set several titles over time; re-attach;
  assert exactly **one** OSC title (the current one), `ST`-terminated, never a stale one.
- **`TestRegression_ReattachNoDanglingSequence`** — feed a chunk that ends mid-OSC (no
  terminator), re-attach; assert the payload is fully terminated and the subsequent live
  stream is **not** swallowed (the old garbled-tab bug) — i.e. a following plain-text write
  appears on screen.
- **`TestRegression_ReattachMidSplitSequence`** — guards the §8.3 mid-sequence attach
  boundary (Issue: orphaned sequence-tail). Feed a chunk that ends **mid-sequence** at a chunk
  boundary (e.g. `…ESC[`, the model buffering the partial), then — **before** the tail chunk
  arrives — `Attach` a fresh client; assert the attach payload **ends with the buffered partial
  bytes** (`redraw + ESC[`, i.e. `PendingInput()` was appended after the ground-state redraw).
  Then deliver the tail (`31mRED`) on the live stream and assert the new client's reconstructed
  buffer shows **red `RED`**, NOT the literal text `31mRED` — i.e. no stream-swallow and no
  orphaned tail. A paired case feeds an over-long un-terminated sequence (> `maxPendingPartial`)
  and asserts `PendingInput()` returns empty and the payload is the clean redraw (the bounded,
  accepted residual). A second paired assertion: `Snapshot()` for the SAME mid-sequence state
  does **NOT** include the partial bytes (the `.buf` stays self-contained — a restored fresh PTY
  would never deliver the tail).
- **`TestRegression_RoundTripIdenticalBuffer`** — render a complex screen (colors, cursor
  off-home, alt-screen, scrollback), serialize, feed into a fresh same-size `xterm`, assert
  the visible buffer is **identical** to the model's. **Includes an origin-mode case** — a
  non-full-screen scroll region with origin mode ON and the cursor off region-home — asserting
  the reconstructed `cursorX`/`cursorY` equal the model's real cursor cell (guards the §6
  step-12-before-step-13 origin ordering + region-relative CUP; a regression where `?6h` is
  emitted *after* the CUP would land the cursor at region-home and fail this).
- **`TestRegression_RoundTripScrollbackDepth`** — guards the step-5/step-6 "no re-home"
  fix (Issue: the old re-home stranded and overwrote the newest `rows` scrollback lines).
  Push **more than `rows`** distinct, individually-identifiable lines through the model so
  the model's scrollback holds at least `rows + k` lines (k ≥ 1); serialize; feed the
  payload into a fresh same-size `xterm`; then assert **(a)** the reconstructed xterm's
  scrollback **line-count equals the model's `ScrollbackLen()`** exactly (not
  `scrollbackLines − rows`), and **(b)** the **newest scrolled-off line** (the one
  immediately above the visible grid's top row) is **byte-identical** to the model's
  newest scrollback line — proving the most-recent history survives attach. A paired case
  drives scrollback to exactly the `scrollbackLines` cap and asserts the count saturates at
  the cap (oldest lines evicted, newest retained). **A third case pushes soft-wrapped long
  lines** (logical lines spanning ≥2 physical rows) and asserts the reconstructed scrollback's
  **physical** line-count and per-row `isWrapped` state match the model's — i.e. wrapped rows
  are NOT flattened into hard breaks (the §6 step-5 wrap-aware separator rule). **A fourth
  case (the grid-row-count Issue) pushes more than `rows` lines AND then parks the cursor
  ABOVE the bottom row so the visible grid has trailing blank rows**, serializes, feeds the
  payload into a fresh same-size `xterm`, and asserts the reconstructed scrollback line-count
  **still equals the model's `ScrollbackLen()` exactly** — proving the grid body emitted all
  `rows` physical rows (trailing blanks as bare `CR`/`LF`, §6 step 7) so the bottom scrollback
  lines scrolled into history rather than stranding in the viewport. A regression that trimmed
  the trailing blank grid rows (e.g. an α `Render()` that trims and is not re-padded to `rows`)
  fails this with a short scrollback count and the newest history surfacing in the viewport.
  **A fifth case is the per-`Write` eviction-ceiling guard (blocker resolution, §9.2): it pushes
  scrollback that MATERIALLY EXCEEDS the former `vtScrollbackWindow` (≥ 2000 distinct,
  individually-identifiable lines — well over 256) and feeds it as ONE single large
  `model.Write`** (a single `>64 KB` chunk, exactly the restore-replay / flood shape that scrolls
  thousands of lines in one call). Assert **(a)** the reconstructed scrollback line-count equals
  the model's `ScrollbackLen()` exactly — i.e. **all** ≥2000 lines were captured by the eviction
  hook, NOT just the most-recent ~256 — and **(b)** the OLDEST surviving line (and the newest) are
  byte-identical to the model's. The **paired cap case** (driving scrollback to exactly the
  `scrollbackLines` cap) is likewise fed via a single large `Write` so the cap is exercised against
  the same one-shot ceiling. The deleted `ScrollbackLen()`-diff drain would fail this case with a
  scrollback truncated to ~256 lines (a hole punched in the middle of history); the mandated hook
  passes because it captures every line in stream order with no per-`Write` ceiling.
- **`TestRegression_SerializeFixpointThroughXVT`** — the round-trip that **persistence
  actually exercises** is serialize → fresh **x/vt** model → re-serialize (x/vt parsing its
  *own* serialized output), not serialize → xterm.js. Drive a complex screen, `Snapshot()`,
  build a fresh `vtModel` at the header size and `model.Write(redraw)`, `Snapshot()` again,
  and assert the two blobs are **byte-identical** (and the two models cell-identical) on the
  clean (notice-free) path. This proves `.buf` is a **stable fixpoint** and cannot silently
  drift across save/restore. **This fixpoint is gated at P0 (§15), not just here** — it is
  load-bearing for *all* persistence, so it must be de-risked before any `model/` code is
  built. **The P0-gated fixpoint fixture MUST exercise scrollback that exceeds a single screen
  and is fed via a single large `model.Write`** (so the eviction hook is driven against the
  one-shot, multi-screen-per-`Write` case the deleted diff drain got wrong — §9.2 / §15 item 2).
  **Fallback if x/vt is NOT a self-fixpoint** (SGR canonicalisation or trailing-space drift on
  re-parse): for the **unchanged scrollback region** the serializer reuses the **verbatim prior
  compact-line bytes** from
  the `lineRing` (which are immutable post-eviction, §9.2) instead of re-deriving them, so
  only the **visible grid** must round-trip stably; and the visible-grid fixpoint is
  enforced by canonicalising SGR/colour emission to a single deterministic form in the
  serializer (already required for determinism, §13.1). If even the grid does not
  self-fixpoint at the pinned commit, that is a P0 **exit-gate failure** that blocks
  adoption (→ evaluate `midterm`, §5.4), not a silent drift discovered at P5.
- **`TestRegression_RestorePreservesGridSize`** — serialize a screen at e.g. 120×40,
  persist, restore via `NewRestored`; assert **(a)** the restore model is built at
  **120×40** (from the §12 header, not a default), **(b)** `pty.Setsize` was called with
  **120×40 before the first `Read`** (assert the restored PTY's window size via
  `pty.Getsize` / `TIOCGWINSZ` equals 120×40, *not* the kernel-default 80×24) **even when
  the re-attaching client supplies no resize**, and **(c)** the fresh shell's first
  post-restore output is parsed at 120×40 (write a width-sensitive line through the
  restored PTY and assert no wrap corruption) — guarding both the "wrong-size restore
  corrupts wrap" hole and the "restored PTY silently stuck at 80×24" hole (§4.2).
- **`TestRegression_ResizeOnlyPersistsNewSize`** — guards the §4.2/§8.4 resize-cache Issue.
  Create a session at 80×24, let it flush once (header says 80×24), then **`Resize(120×40)`
  with NO subsequent PTY output**, and trigger a cadence `flushSessionOnce`. Assert **(a)**
  `Snapshot()` returns `changed == true` (the resize marked dirty and nil'd `lastBlob`, so the
  cache did NOT return the stale 80×24 blob), **(b)** the freshly persisted `.buf` `CRWB1`
  header is **120×40**, not the pre-resize 80×24, and **(c)** restoring that `.buf` via
  `NewRestored` builds the model and `Setsize`s the PTY at **120×40** (chains into
  `RestorePreservesGridSize`). Without the resize dirty-mark+cache-invalidation this fails at
  (a)/(b) — the cache hands back the old-size blob and the wrong-size-restore corruption
  returns through the caching layer.
- **`TestRegression_ReattachAfterKilledAltScreenApp`** — **table-driven over all three
  alt-screen entry variants (`?1049h`, `?47h`, `?1047h`)**: start an app, enter alt-screen
  via the variant + enable mouse (`?1000h`), **SIGKILL** it (no reset emitted), let the
  foreground pgrp return to the shell; re-attach and assert the payload contains **no**
  `?1049h` / `?47h` / `?1047h` / `?1000h` and serializes the **primary** (shell) screen with
  scrollback (step 5 present, not the alt branch) — i.e. §11.1's teardown took x/vt out of
  the alt buffer **regardless of which DECSET variant entered it** and the payload is
  internally consistent (the old killed-alt-screen leak, and the `?47h`/`?1047h` "x/vt stuck
  in alt while shadow says primary" contradiction, do not return).
- **`TestRegression_ForceSuspendAltScreenThenRestore`** — guards the §11.2 suspend-of-alive
  contract. Start an app, enter alt-screen (`?1049h`) + enable mouse (`?1000h`), paint alt
  content, and — **while the app is still the live foreground process group (no app→shell
  edge)** — `BeginForceSuspend` the session, then restore it via `NewRestored`. Assert the
  persisted `.buf` and the restored payload serialize the **primary** buffer (scrollback
  present / step 5 fired, **no** `?1049h`/`?47h`/`?1047h`, **no** `?1000h`) and the restored
  screen is **clean** (the shell prompt renders, no frozen alt frame, no leaked mouse/alt
  residue) — i.e. force-suspend ran the §11.2 model teardown before `Snapshot()` so a
  suspended-while-alive alt app does not strand the restored client in a frozen alt buffer.
  Also asserts a force-suspend notice injected via `InjectLocal` lands in the **primary**
  screen (§12/§11.2), never the alt buffer.
- **`TestRegression_ForceSuspendInterleaveRace`** — guards the §11.2 atomicity Issue (a live
  `pumpStep` chunk interleaving between teardown and serialize). Run with `-race`. Drive an
  app into alt-screen (`?1049h`) + mouse, and force-suspend it via the
  `forceSuspendSnapshotLocked` path while **a concurrent goroutine repeatedly feeds an
  alt-repaint chunk through `PumpChunkForTest`** (a periodic frame that does NOT re-emit
  `?1049h`, exactly as htop/vim would). Made deterministic with a hooked model whose
  `OnForegroundReset` blocks on a barrier the test releases only **after** it has launched the
  concurrent chunk — proving the chunk must wait on `s.mu` (the single combined hold) and
  cannot land between teardown and serialize. Assert the persisted blob is **primary** (no
  `?1049h`, scrollback/step-5 present) on **every** iteration regardless of the racing chunk,
  and that `BeginForceSuspend` persists the blob returned by `forceSuspendSnapshotLocked`
  **without re-`Snapshot()`ing** (a post-hold chunk cannot change the persisted bytes). The
  three-acquisition form (separate `OnForegroundReset`/`InjectLocal`/`Snapshot`) would fail
  this non-deterministically; the single-hold form passes deterministically.
- **`TestRegression_LivePathUnchangedRaw`** — assert `pumpStep` fans out the **raw** chunk
  byte-for-byte (no transformation) and the **full ordering** via a hooked model + a hooked
  foreground sampler: fan-out happens **before** the model write, **and** the debounced
  `checkForegroundResetLocked()` (the `TIOCGPGRP` sample and any teardown `model.Write`) runs
  **strictly LAST — after both `fanOutLocked` and `writeModelLocked`** (§8.2 step (3), §11.1
  site #1) — so neither the ioctl nor the teardown write can ever precede or delay the live
  fan-out. Guards NG1.
- **`TestRegression_ReattachDuringAltScreenThenExit`** — guards the §10(b)/G3 alt-screen
  carve-out as an asserted, non-corrupting outcome. Enter alt-screen (e.g. vim) and paint
  alt content; re-attach a fresh client (gets `?1049h` + alt grid, no scrollback); then drive
  the app to **exit** (`?1049l` on the live stream). Assert the post-exit **primary** buffer
  on the re-attached client is **clean/empty (never garbage)** — the documented accepted loss
  of the pre-app shell screen + scrollback — and that the subsequent shell prompt renders
  cleanly with no leaked alt/mouse residue. This pins the one fidelity gap as a known,
  bounded, non-regressing behavior rather than silent corruption.
- **`TestRegression_ResizeDuringAltScreenNoPollution`** — guards the §9.2/§10(a) blocker Issue
  (alt-buffer rows leaking into primary scrollback on a resize). Drive a full-screen app into
  alt-screen (`?1049h`) and fill the alt grid; capture the model's primary compact-ring
  line-count + `sbBytes`; **resize (height-SHRINK)** the session while still in alt-screen;
  assert the primary ring line-count and `sbBytes` are **unchanged** (the displaced alt rows did
  not enter the primary ring). Then drive the app to exit (`?1049l`), re-attach, and assert the
  serialized **primary** scrollback is clean — byte-identical to the pre-alt primary, with **no**
  alt rows present. Without the `!altScreen` drain guard the shrink pollutes the primary ring and
  this fails.
- **`TestRegression_PartialScrollRegionNoDrain`** — guards the §9.2 major Issue (DECSTBM
  partial-region scroll mis-draining). Set a **partial** scroll region (`ESC[2;23r` on a 24-row
  grid, reserving a status line) and drive content that scrolls **inside** the region; assert the
  model's compact ring `sbBytes` is **unchanged** (region-internal scrolled-off lines are
  discarded, not stored) and that a re-attach serializes the region/grid correctly with the
  reserved line intact. Then clear the region to full-screen (`ESC[r`) and assert subsequent
  scrolls **do** drain into the ring — proving the drain is gated on the full-screen condition,
  not unconditional.
- **`TestRegression_ReattachAdversarialTitle`** — guards the §6 step-16 `sanitizeOSCText`
  hardening. Set a window title via `OSC 2` whose text contains an **embedded `ESC[…m`** and
  whose terminator is **missing/garbled**; also set an icon name with embedded C0 bytes.
  Re-attach and assert the payload's title OSC is **single, well-formed, `ST`-terminated, and
  control-char-free**, and that a plain-text write issued immediately after the redraw
  **appears on screen** (is not swallowed into a never-terminated title). Guards against
  re-introducing the garbled-tab / stream-swallow class through app-controlled title text.
- **`TestRegression_IdleSessionNoCadenceWrite`** — guards the §8.4 write-skip signal. Flush a
  session once (blob written), then with **no further output** trigger a cadence
  `flushSessionOnce`; assert `Snapshot()` returns `changed == false` and that the tick
  performs **no `WriteBuf` and no `saveMeta`** (hooked persistence layer records zero calls),
  i.e. an idle session causes zero cadence disk I/O — matching today's dirty-gated behavior
  and preventing the 100-sessions-every-10s write regression. A paired assertion: after one
  byte of output the next tick **does** write (`changed == true`).
- **`TestRegression_ModelPanicOnResizeAndAttachStillServes`** — guards the §8.5 panic-isolation
  boundary (Issue: recover only covered the `Write` path). Using the `newEmulator` seam (§4.1) to
  inject an emulator that panics on a sentinel, drive a panic on **(1)** the `Session.Resize`
  drain path and **(2)** the `Attach`→`serializeLocked` path. Assert each **bumps `modelPanics`**,
  does **NOT** kill the session (no `shutdown`, the `done` channel stays open), and the session
  still serves a subsequent `Attach` and `Write`. Without the §8.5 `serializeLocked`/
  `mutateModelLocked` wrappers the Resize panic crashes the readPump goroutine and the Attach
  panic unwinds to `pump()`'s `safego.Recover → s.shutdown()`, killing the session — this test
  fails in that regression.
- **`TestRegression_CacheDropReclaimsBeforeSuspend`** — guards the §9.4 Phase-3 pre-step (Issue:
  the `lastBlob` reclaim was prose-only). Bring the engine over `maxTotalModelBytes` with several
  live sessions each holding a reclaimable `lastBlob`; run `runMaintenanceOnce` with hooked
  `Suspend`/`evict` counters. Assert the Phase-3 pre-step calls `DropCachedBlob()` on the
  LRU-coldest live sessions and returns **under the ceiling with ZERO `Suspend`/`evict` calls**,
  and that each dropped session has `lastBlob == nil` && `dirty == true`. A paired case makes the
  caches insufficient (the resident model bytes alone exceed the ceiling) and asserts the loop
  **does** proceed into Phases 3a/3b/3c after the pre-step — proving the pre-step is a reclaim
  gate, not a replacement for suspend/evict.
- **`TestRegression_RestoreNoticeBeforeAttach`** — guards the §12/§9.1 restore-notice ordering
  (Issue: post-`spawn` injection raced `reg.Add`). Restore a placeholder whose restore carries a
  `[crowbar] restored …` notice; with a hooked engine `spawn` that lets the test attempt an
  `Attach` the instant `reg.Add` returns, assert the **first** attaching client's serialized
  payload **already contains the notice** (it was injected before `reg.Add`), never a payload
  missing it. Run with `-race`.
- **`TestRegression_PumpStepRaceFree`** — retained race-detector test via `PumpChunkForTest`
  with concurrent `Attach`/`Serialize`.

### 13.4 Path to 100% coverage on new packages

- `model/` has **no IO and no goroutines** → every branch is reachable from a table-driven
  unit test. The `coverpkg` gate (repo convention) includes `model/`.
- Error/edge branches (empty model, zero-size resize, scrollback at exactly cap, ignored
  modes) each get an explicit case.
- The adapter's "callback fired" branches are covered by feeding the PTY byte sequences
  that trigger each callback through `Write` (e.g. an `OSC 2` sets `title`; `?1049h` sets
  `altScreen`), so coverage is driven through the real parse path, not by calling callbacks
  directly.

---

## 14. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **Untagged `x/vt`/`ultraviolet` API churn** (2-week-old library). | Med | Med | Pin exact commits (§5.3); quarantine behind interface (§4); conformance harness gates every bump; `midterm` is an **interface-shaped** drop-in for the buffer-equivalence surface, but its three HARD-GATE capabilities (eviction-hook+wrap, per-row wrap signal, byte-identical self-fixpointing row encoder, §15 items 6/7) are **unverified** until P0 sub-task `M` (§5.4.1) runs — with a committed tertiary plan (portable scroll/eviction wrapper → fixpoint fallback → third emulator) if midterm refutes them, so the fallback is no longer presented as a guarantee. |
| **Serialize fidelity diverges from `xterm.js`** (wide glyphs, ZWJ, scroll regions). | Med | Med | `SerializeAddon` conformance harness with CJK/emoji-ZWJ fixtures (§13.2) as a hard gate; deviation is *only* the documented OSC title. |
| **`x/vt` not thread-safe** → data race. | Low | High | All model access under `s.mu` (§8); race-detector test retained; documented `SafeEmulator` escape hatch (one-line, file-local). |
| **`model.Write` panics on adversarial PTY bytes → corrupted parser and/or `s.mu` poisoned / permanent deadlock** (untagged ~2-week-old emulator that buffers partial escape/parser state across Writes; an explicit `Unlock` would be skipped on panic, stranding the lock; and a mid-parse panic can leave the parser mis-parsing ALL later chunks, not merely one behind). | Low | High | **Substantive recovery in the adapter (§4.1):** on a parse panic `vtModel.Write` RECREATES the emulator to a known-blank, fully-parseable state at the current size (the "one chunk behind, repaint reconciles" claim is removed — a corrupted parser is assumed), sets a sticky `degraded` flag, bumps `parsePanics`, and PRESERVES the compact scrollback ring. The model is guaranteed parse-correct afterward; the app repaints next frame; live clients already got the raw bytes. **Lock safety + ALL paths (§8.5):** `defer s.mu.Unlock()` in `pumpStep` (never an explicit `Unlock`) plus the `recover` in `writeModelLocked` (§8.2) cover the Write path; the **other model-access paths** — `Serialize` (Attach/Snapshot/force-suspend, incl. the §4 downcast panic), `Resize`'s eviction hook (`drainEvictedLine`), `OnForegroundReset`'s teardown, and `injectLocalLocked`'s notice writes — route through `serializeLocked`/`mutateModelLocked`/`writeModelLocked` (§8.5) so a panic on any of them ALSO bumps `modelPanics` and leaves the session serving instead of escaping to `pump()`'s `safego.Recover → shutdown` or the readPump goroutine. Covered by `model/` unit tests asserting (i) the session still serves the next `Attach`, and (ii) **model CORRECTNESS after recovery** — post-recovery Writes parse cleanly and a Serialize→fresh-model round-trip reproduces the post-recovery screen, with `degraded` set (panic injected via a test-only emulator seam) — plus `TestRegression_ModelPanicOnResizeAndAttachStillServes` (§13.3) for the Resize/Serialize paths. |
| **Memory regression** (cell-grid scrollback blowup, or x/vt keeping its *own* deep scrollback). | Low | High | x/vt built with `WithScrollback(0)` — **zero** retained cell scrollback (the mandatory eviction hook captures each line, §9.2), **never** `scrollbackLines`; authoritative depth is the compact-line ring; `ModelBytes()` **counts the (zero) x/vt window** so the ceiling is honest (§9.4); budget bounds peak at ~100–180 MB (§9.3). |
| **Model update adds live latency under flood.** | Low | Med | Fan-out-first ordering (§8.2): model write strictly *after* sends, pure CPU, no IO; can never delay an already-enqueued byte. |
| **`x/vt` lacks a needed callback** (OSC 7 cwd, charset). | Med | Low | Documented fallbacks: `parseLastOSC7` (kept) for cwd; in-`Write` `ESC ( / )` scan for charset. Each resolved + recorded in the P0 spike (§15). |
| **`x/vt` lacks the pre-scroll / line-eviction hook** (evicted-line + wrap capture). | Low | High | **NO software fallback** — the deleted `ScrollbackLen()`-diff drain has a per-`Write` capture ceiling (≤256 lines/Write) that loses earlier evictions on a restore replay / flood / scroll-sequence (§9.2). The hook is a **HARD P0 adoption gate** (§15 item 7): absent it (and no thin wrapper around the scroll path), x/vt is NOT adopted → `midterm` (§5.4). x/vt is always built `WithScrollback(0)`. |
| **Stale modes leak into an idle shell** after a SIGKILLed alt-screen/mouse app. | Med | Med | Foreground-pgrp teardown edge (§11.1): `OnForegroundReset` feeds a model-only canonical teardown + `resetTransientModes()`; `TestRegression_ReattachAfterKilledAltScreenApp` (§13.3) guards it. |
| **`.buf` restored at the wrong grid size** corrupts wrap. | Low | High | §12 size header (`CRWB1 cols rows alt scrollback`); restore builds the model at the persisted size; `TestRegression_RestorePreservesGridSize` + `TestRegression_SerializeFixpointThroughXVT` (§13.3) guard it. |
| **Serialize-on-attach perf for huge scrollback.** | Low | Low | Scrollback is pre-serialized (stored as bytes), so step 5 is a memcpy, not a render; only the visible grid is rendered on attach. |
| **Soft-wrapped lines flattened into hard breaks** (divergence from `SerializeAddon` line-count/wrap state). | Med | Med | Per-row `wrapped` flag read **live from the backend** — the eviction hook for scrollback rows, `RowWrapped` for visible grid rows — and stored with each compact line; step 5 omits the `\r\n` between a wrapped row and its continuation; wrapped rows encoded full-width (§6 steps 5/7, §9.2). The wrap signal is a **HARD P0 adoption gate** — no software side-array fallback (it would require re-parsing the byte stream); absence → `midterm` (§5.4, §15 exit-gate item 7). §13.2 `softwrap` fixture is the gate. |
| **Origin-mode cursor lands at region-home** (DECOM `?6h` re-homes the cursor; emitting it after the CUP destroys placement). | Low | High | `?6h` asserted in **step 12, BEFORE** the step-13 CUP, and the CUP emitted region-relative when origin mode is ON (§6). Guarded by `TestRegression_RoundTripIdenticalBuffer`'s origin-mode case + the §13.1 golden-byte origin case. |
| **Force-suspending a LIVE alt-screen app freezes the restored screen** (no app→shell edge fires, frozen alt grid persisted). | Med | High | Force-suspend treated as an app-death edge: `OnForegroundReset` runs BEFORE the suspending `Snapshot()`, dropping to the clean primary (§11.2); `TestRegression_ForceSuspendAltScreenThenRestore` guards it. |
| **Per-line encoder not byte-identical to grid `Render()`** → `.buf` self-fixpoint breaks. | Med | High | `encodeRow` reuses ultraviolet's single-row render (or a canonical SGR-RLE fallback) and is P0-gated byte-identical to grid-row `Render()` output (§9.2.1, §15 exit-gate item 6); failure blocks adoption (→ `midterm`). |
| **`DECSTR` does not force autowrap ON** (implementation-defined) → grid paints with wrong wrap. | Low | Med | §6 step 1 emits an **explicit `?7h`** after DECSTR regardless of DECSTR's DECAWM behavior; P0 records the pinned-commit behavior but correctness no longer depends on it. |
| **Cached `lastBlob` pushes resident memory over the ceiling.** | Low | Med | `lastBlob` counted in `ModelBytes()` (honest) but reclaimed FIRST under pressure (drop = nil, re-serialized on demand) before any suspend/evict (§9.3/§9.4), so it never forces spurious churn. |

---

## 15. Phasing

**P0 — Spike (de-risk the untagged emulator).** Add pinned `x/vt` + `ultraviolet` +
`x/ansi`. In a throwaway `main`/test, build a `vtModel`, feed a captured `vim` PTY stream,
call `Render()` + hand-built prologue/epilogue, and diff against `SerializeAddon`. Confirm
the §4.1 contract-note capability questions, each of which has a committed fallback so the
design is not blocked on the answer:
- exact `vt.Callbacks` field names/signatures;
- **scrollback drain mechanism (§9.2) — SINGLE mechanism, HARD adoption gate (not a "pick one"
  spike question).** The pre-scroll/eviction hook (→ `WithScrollback(0)`, retained-window 0) is
  the **only** production mechanism; confirm x/vt exposes it (a `Callbacks` entry or a thin
  wrapper around its scroll path delivering each line's cells + wrap bit before x/vt drops it). The
  `ScrollbackLen()`-diff alternative is **DELETED** as unsound — it captures at most ~256 evictions
  per `Write`, losing the rest on a restore replay / flood / `CSI Ps S` (§9.2 "Drain mechanism").
  **There is no "settle for both outcomes": the budget (§9.3) has one outcome (retained-window 0).**
  If the hook is absent and no wrapper can synthesise it, adoption BLOCKS → `midterm` (exit-gate
  item 7). The spike records the hook's exact field name in `model/UPSTREAM.md`;
- OSC 7 working-directory callback presence (else `parseLastOSC7` fallback);
- charset/SCS callback presence (else in-`Write` `ESC ( / )` scan → `shadow.g0/g1`);
- **locking-shift (`SI`/`SO`) observability** — a `LockingShift` callback, else an
  in-`Write` `SI`/`SO` scan → `shadow.glLock` (drives step 10 GL invocation, §6 Issue);
- whether `Render()` re-emits **OSC 8** hyperlinks (→ survive vs accepted-un-restored, §6.2);
- **default-color set-signal (step 2, §4.1)** — an `OSC 10/11/12` callback, else an
  in-`Write` OSC scan, else (last resort) **omit step 2 entirely**; confirm which applies
  and that a never-recoloured app emits **no** OSC 10/11/12 (never overrides the client
  theme). The bare getters alone are NOT sufficient and must not be used to gate step 2.
- **pending-wrap latch observability (§6.2 / step 13)** — whether x/vt exposes the
  deferred-autowrap state; if yes the last-column repaint re-arms it, if no it is the
  bounded one-cell residual.
- **parser ground-state / buffered-partial exposure (§4.1/§8.3) — drives the mid-sequence
  attach re-sync.** Confirm whether the pinned x/vt commit exposes its parser's
  ground-state predicate or its buffered partial-sequence bytes. **Preferred:** `PendingInput()`
  mirrors it directly. **Fallback (always available):** the adapter runs its own ECMA-48
  escape-**framing** tracker in `Write` (`trackPendingPartial`, §4.1) — recognising sequence
  framing only, never semantics — carrying the in-flight bytes across chunks, capped at
  `maxPendingPartial`. Absence of an x/vt hook is therefore **NOT** an adoption blocker for the
  attach re-sync; the spike only records which source is used. Asserted by the §13.1
  `trackPendingPartial`/`PendingInput` unit test and §13.3 `TestRegression_ReattachMidSplitSequence`.
- **soft-wrap (`isWrapped`) exposure (§6 steps 5/7, §9.2) — HARD ADOPTION GATE (blocker
  resolution).** Confirm that ultraviolet/x/vt exposes a per-row wrap signal usable as the SOLE
  wrap-flag source, split by row kind because the two kinds have different ceilings and **BOTH**
  are required: **(i-a) EVICTED scrollback rows — the pre-scroll/eviction hook, REQUIRED** (no
  getter alternative): it delivers each evicted line's `wrapped` bit **alongside its cells** at
  eviction time, with no per-`Write` ceiling. (The previously-listed retained-window getter
  `ScrollbackRowWrapped(y)` + `ScrollbackLen()`-diff is **deleted**: a window getter could read at
  most ~256 of the lines a big `Write` evicts — the same hole the line-capture diff had.) **(i-b)
  VISIBLE grid rows — `RowWrapped(y)` readable on demand** (or a `Render()` that emits a wrapped
  grid row's content **continuously with no `CR`/`LF`**, relying on autowrap, rather than forcing
  a hard break). The visible grid is bounded `rows×cols` with no eviction ceiling, so a getter is
  fine here.
  **There is NO software-fallback for the wrap flag.** The previously-considered
  adapter-maintained `gridWrap` side array is **deleted as unsound**: the adapter calls
  `emu.Write(p)` (x/vt owns the parse) and reads getters only at chunk boundaries, so it cannot
  reconstruct which rows soft-wrapped before eviction in a multi-screen-per-chunk flood — doing
  so would require re-implementing x/vt's cursor/glyph-width/scroll-region state machine inside
  the adapter (§9.2 "Why there is NO adapter-maintained side array"). **Therefore: if the pinned
  x/vt commit exposes no eviction hook (i-a) — or no usable visible-grid wrap signal (i-b) — and no
  thin wrapper around its scroll path can recover one, x/vt is NOT adopted — this is a P0 ADOPTION
  BLOCKER, and we fall to `midterm` — but ONLY after **P0 sub-task `M` (§5.4.1)** verifies midterm
  satisfies the SAME hard gates (or the §5.4.1 tertiary plan's portable scroll/eviction wrapper
  supplies them), since midterm is NOT pre-vetted for these specific capabilities — then re-run the
  spike against the chosen backend.** The §13.2 `softwrap` fixture (line-count + wrap state vs the `SerializeAddon` oracle) is
  the gate that the chosen backend's wrap signal must pass. (`Render()`'s own wrapped-grid-row shape is a
  separate question, handled by the α/β branches of §15's `Render()`-shape item and §9.2.1's
  `encodeRow`.)
- **single-row encoder byte-identity (§9.2.1) — load-bearing for the `.buf` fixpoint.**
  Confirm whether ultraviolet exposes a **public single-row render entrypoint** so
  `encodeRow` can produce bytes **byte-identical** to the grid `Render()`'s per-row output
  (the primary mechanism). **Fallback:** a hand-rolled canonical SGR-RLE `encodeRow` that the
  P0 gate REQUIRES to be byte-identical to the grid-row `Render()` output; if it cannot be
  made identical at the pinned commit, the self-fixpoint breaks for the visible grid and
  adoption is blocked (→ `midterm`).
- **x/vt resize-eviction semantics (§4.1 `Resize` / §10(a)) — the eviction hook MUST fire on
  shrink.** Confirm a height **shrink** pushes the overflowing top rows off the top **through the
  same eviction hook** that a scroll uses (so they are captured into the compact ring during
  `emu.Resize`), and that a height **grow** does NOT repopulate the grid (x/vt is
  `WithScrollback(0)`, so there is no retained window to pull from — grow just adds blank rows).
  **No diff fallback exists** (deleted, §9.2): if the pinned commit's shrink path bypasses the
  eviction hook, a **thin wrapper around `emu.Resize`** must re-route the displaced rows through
  `drainEvictedLine`; if neither the hook nor a wrapper captures shrink-displaced rows, adoption
  blocks → `midterm`. Asserted by the §13.1 resize unit tests.
- **x/vt WIDTH-change live-grid behavior (§4.1 `Resize` / §9.2 "Width-change resize" /
  §10(a)) — informational, not a wrap-flag concern.** Confirm whether x/vt **reflows the LIVE
  visible grid on a width change** (re-wraps existing rows to the new column count) or merely
  **clips/extends** each row. Either way the design needs **no** wrap-flag fix-up on resize:
  there is no adapter side array to rebuild (blocker resolution, §9.2), and every row's
  `wrapped` bit is read live from the backend at paint (`RowWrapped`) or delivered by the eviction
  hook, so it always reflects x/vt's **current** post-resize wrap
  state. This item is recorded only so the §13.1 width-shrink/grow-then-evict unit test scripts
  the seam's post-resize wrap signal to match whichever behavior the spike observes. Already-
  evicted compact lines are immutable (NG2) and were stamped at their own eviction time, so they
  are never retroactively stale.
- **DECSTBM partial-scroll-region → scrollback semantics (§9.2 / §4.1) — load-bearing for ring
  integrity.** Confirm that x/vt pushes a scrolled-off top line into scrollback **ONLY** when
  the scroll region spans the full screen (`scrollTop==1 && scrollBottom==rows`), and that a
  scroll **inside an active partial region discards** the displaced line (does NOT grow
  `ScrollbackLen()` and does NOT fire the pre-scroll hook). This is standard VT/DECSTBM
  behavior; the spike pins it for x/vt. The design **does not depend on the answer being
  permissive**: the `drainEvictedLine` hook handler is gated on the full-screen condition, so even
  if x/vt fired the hook for a region-internal scroll the guard prevents it from capturing a
  discarded line. There is no side-array shift to confine or mis-apply (blocker resolution, §9.2) — a
  visible row whose wrap state changes inside the region is reported correctly on the next paint
  because `RowWrapped` reads x/vt live. Asserted by `TestRegression_PartialScrollRegionNoDrain`
  (§13.3) and the §13.1 full-screen-vs-partial-region drain unit test.
- **DECSTR effect on DECAWM (§6 step 1).** Confirm whether DECSTR (`ESC[!p`) forces autowrap
  ON at the pinned xterm.js/x-vt commits. **This is informational only:** §6 step 1 emits an
  **explicit `?7h`** after DECSTR regardless, so the autowrap-ON baseline is correct even if
  DECSTR leaves DECAWM untouched. (Recorded so the explicit `?7h` is not later "optimized" away.)
- **`Render()` output SHAPE (load-bearing for steps 5–7) — empirically confirm at the
  pinned commit that `ultraviolet.Render()` (via `x/vt.Render()`):** (i) emits **NO
  absolute cursor-positioning** — no `CUP`/`HVP` (`ESC[…H`/`ESC[…f`), no `CUU`/`CUD`/`CUF`/`CUB`,
  no bare home `ESC[H`, no `CHA`/`VPA`; (ii) advances row-to-row via `CR`/`LF` for **hard**
  line breaks while emitting a **soft-wrapped** row's content continuously (no `CR`/`LF`,
  autowrap carries it); (iii) emits **NO trailing newline** after the final grid row; **and
  (iv) emits EXACTLY `rows` physical rows — trailing blank rows included as bare `CR`/`LF`,
  NOT trimmed (the row-count requirement, §6 step 7).** The
  entire §6 scrollback-restoration mechanism (steps 5–7 as one continuous flow, the deleted
  step-6 re-home, and "the trailing `rows` rows are the visible grid") **depends on this shape
  and asserts it as fact** — it is NOT covered by the alt/vim fixture (which has no
  scrollback), and the row-count half (iv) is NOT covered by a full-grid fixture, so the gate
  fixture parks the cursor above the bottom row. If `Render()` does **not** have this shape,
  the committed fallback (recorded before P1) is one of:
  - **(α) adapter post-process** — strip any absolute moves, rewrite hard row advancement to
    `CR`/`LF`, omit the separator for wrapped rows, trim any trailing newline, **and re-pad to
    exactly `rows` physical rows if `Render()` trimmed trailing blank rows** (append the
    missing bare `CR`/`LF` line breaks), before it becomes step 7's body. Steps 5–7 are
    otherwise unchanged. If α cannot reliably massage the shape (including the re-pad), fall
    to β.
  - **(β) own per-row grid paint (FULLY SPECIFIED).** If `Render()`'s shape cannot be massaged
    by α (e.g. it interleaves absolute moves mid-row), **do NOT use `Render()` for the grid
    body at all.** Instead emit the visible grid with the **same §9.2.1 `encodeRow`** used for
    scrollback, so the grid body has the identical `CR`/`LF`-for-hard-breaks +
    autowrap-for-soft-wraps shape by construction. The complete §6 byte sequence under β is
    **identical to α except step 7's body**:
    - Steps **1–4** unchanged (DECSTR + explicit `?7h`, colors, alt-enter, clear+home).
    - Step **5** unchanged — stored scrollback rows via `encodeRow`, wrap-aware `\r\n`
      (separator omitted after a `wrapped` row), continuing the one flow.
    - Step **6** unchanged — **no re-home** (the deleted step).
    - Step **7 (β form)** — walk the `rows` visible grid rows top-to-bottom; for each row read
      its `uv.Cell`s via `CellAt` and emit `encodeRow(cells, rowWrapped)`; between rows emit
      `\r\n` **only when the upper row is NOT soft-wrapped** (a soft-wrapped grid row's
      continuation follows with no separator, autowrap carrying it); emit **no trailing
      newline** after the final row. The grid thus continues the step-5 flow exactly as α's
      `Render()` body would, so its `rows` rows are the trailing/visible lines and **all**
      scrollback rows land in xterm history — **scrollback line-count fidelity is preserved
      without any re-home or absolute positioning** (this is why β paints the grid in-flow
      rather than as a separately re-homed absolute paint, which could not preserve the count
      without polluting history with blank lines).
    - Steps **8–17** unchanged.
    β reuses one already-specified primitive (`encodeRow`) and adds no new positioning logic;
    both α and β are therefore fully byte-level specified before P1 regardless of the spike
    outcome. The chosen branch is recorded in `model/UPSTREAM.md`.

Record findings + commit pins in `model/UPSTREAM.md`. **Exit gate (all required):**
1. one fixture round-trips **buffer-identical** vs `SerializeAddon`;
2. the **x/vt serialize → fresh-x/vt → re-serialize self-fixpoint** holds **byte-identical**
   on a fixture exercising **scrollback + visible grid + active modes** — and, mandatorily, **a
   scrollback that MATERIALLY EXCEEDS a single screen (≥ 2000 lines, well over the former 256-line
   window) fed via a SINGLE large `model.Write`** (the restore-replay / flood shape, blocker
   resolution §9.2) so the eviction hook is exercised against the multi-screen-per-`Write` ceiling
   the deleted diff drain got wrong (the `TestRegression_SerializeFixpointThroughXVT` +
   `RoundTripScrollbackDepth` shapes, §13.3). Assert the reconstructed scrollback line-count equals
   the model's `ScrollbackLen()` exactly (no truncation to ~256). Persistence is load-bearing on
   this, so it is de-risked **before P1**, not at P5; if it fails, the §13.3 fallback (verbatim
   prior compact-line reuse + canonical SGR) must be shown to recover it, else adoption is blocked
   (→ `midterm`);
3. the eviction hook is confirmed present (the SOLE drain mechanism) and the retained-window
   constant is recorded as **0** (`WithScrollback(0)`) so §9.3/§9.4 accounting is locked to its
   single outcome; if no hook (and no wrapper) exists, adoption blocks (item 7) — there is no
   diff-fallback constant to record;
4. the step-2 set-signal and locking-shift mechanisms (callback vs in-`Write` scan vs omit)
   are recorded so §6 steps 2 and 10 are unambiguous before P1;
5. **`Render()` output shape AND row-count are empirically verified and recorded in
   `model/UPSTREAM.md`:** on a fixture **with scrollback** AND **the cursor parked ABOVE the
   bottom row so the visible grid has trailing blank rows** (NOT the alt/vim fixture, whose
   grid is full), confirm `Render()` (i) emits **no absolute cursor-positioning** (no
   `CUP`/`HVP`/`CUU`/`CUD`/`CUF`/`CUB`/`CHA`/`VPA`/bare `ESC[H`), (ii) advances via `CR`/`LF`
   for hard breaks, (iii) emits a **soft-wrapped** row's content continuously with no
   `CR`/`LF`, (iv) emits **no trailing newline**, **and (v) — the row-count sub-assertion
   (Issue) — emits EXACTLY `rows` physical rows, trailing blank rows included as bare
   `CR`/`LF`, NOT trimmed.** Sub-assertion (v) is mandatory because a trimmed grid body short
   by the trailing-blank count would strand the newest scrollback lines in the viewport
   instead of history (§6 step 7) — the P0 gate that runs on a scrollback fixture but does
   NOT count rows cannot distinguish a safe α from an unsafe one, so it is added here. **If
   `Render()` trims trailing blank rows (as `SerializeAddon` does), the decision is committed:
   mandate the α post-process to re-pad the body to exactly `rows` rows (append the missing
   bare `CR`/`LF` line breaks) or fall to β** (β walks all `rows` rows and is exact by
   construction). Record which applies in `model/UPSTREAM.md`. If the shape otherwise differs,
   the chosen fallback (adapter post-process **α**, or the fully-specified
   own-per-row-encoder grid paint **β**) is recorded **before P1** — steps 5–7 must not be
   built on an unverified `Render()` shape. **Both α and β are now fully byte-level specified
   above, so either outcome is buildable;**
6. **single-row `encodeRow` byte-identity (§9.2.1)** is verified: the per-evicted-row /
   per-grid-row compact encoding is **byte-identical** to the grid `Render()`'s output for the
   same row, and self-fixpoints through x/vt. If the hand-rolled fallback cannot match the
   grid-row bytes at the pinned commit, the visible-grid fixpoint fails → adoption blocked
   (→ `midterm`). This gate is what makes the compact-scrollback + `.buf` persistence sound;
7. **eviction hook + soft-wrap fidelity (§6 steps 5/7, §9.2) — HARD ADOPTION GATE.** The chosen
   backend MUST expose **both** (i-a) a **pre-scroll/eviction hook** delivering each evicted line's
   cells + `wrapped` bit (the SOLE scrollback-capture + evicted-row wrap source, with no per-`Write`
   ceiling — `WithScrollback(0)`), **and** (i-b) a usable visible-grid wrap signal — `RowWrapped(y)`
   or a `Render()` that emits wrapped grid rows continuously. **There is no software fallback:** the
   deleted `ScrollbackLen()`-diff + retained-window-getter path loses >256-evictions-per-`Write`,
   and the unsound `gridWrap` side array cannot reconstruct wrap state from chunk-boundary getters
   in a multi-screen-per-chunk flood (§9.2). **If x/vt exposes no eviction hook (i-a) — or no
   visible-grid wrap signal (i-b) — and no thin wrapper can recover one, adoption is BLOCKED → run
   P0 sub-task `M` (§5.4.1) to vet `midterm` against these SAME hard gates (it is NOT pre-vetted for
   them), then the §5.4.1 tertiary plan (portable scroll/eviction wrapper → fixpoint fallback →
   third emulator) if midterm also refutes one; adopt whichever backend the chosen path makes pass
   and re-run this gate against it.** With the signals present, verify on the
   §13.2 `softwrap` fixture that the stored compact line carries its `wrapped` flag, step 5 omits
   the `\r\n` between a wrapped row and its continuation, and the reconstructed buffer matches
   `SerializeAddon`'s **line-count and wrap state** for a build-log/`less` long-line capture;
8. **resize-eviction (§4.1/§10(a))** is verified: the eviction hook **fires for height-shrink-
   displaced rows during `emu.Resize`** so they are captured into the compact ring (correct count +
   `sbBytes`), and a height-grow neither un-captures nor double-counts (no retained window to pull
   from); if the shrink path bypasses the hook, a thin `emu.Resize` wrapper re-routes the rows
   through `drainEvictedLine`, else adoption blocks;
9. **alt-screen / partial-region / width-change drain integrity (§9.2)** is verified: a
   height-shrink **while in the alt buffer** leaves the primary compact ring + `sbBytes`
   **unchanged** (alt rows never pollute primary scrollback); a scroll inside an **active partial
   DECSTBM region** does **not** drain (only a full-screen scroll/shrink does); and a **width
   change** introduces **no** stale wrap flag — because the `wrapped` bit is delivered by the
   backend's eviction hook at eviction time, there is no cached side array
   to recompute, and a row evicted after the resize carries the backend's current flag — the
   guards the blocker/major Issues added, each asserted by the §13.1 unit tests and the §13.3
   `ResizeDuringAltScreenNoPollution` / `PartialScrollRegionNoDrain` regressions.

**P1 — Build the `model/` package.** Implement `TerminalModel`, `Serializer`, `shadowState`,
the `vt` adapter, and `model.New`. Full unit suite to **100% coverage** + the conformance
harness across all fixtures. No session wiring yet. **Exit gate:** harness green on all
fixtures; coverage gate green.

**P2 — Dual-run (shadow mode, no behavior change).** Wire `s.model` into `Session`
alongside the existing ring: `pumpStep` writes **both** (fan-out raw, write model, *also*
write ring); `Attach` still uses the old replay. Add an internal assertion/log diffing the
model serialize vs the ring replay in tests. Validates the model tracks real sessions live
without changing user-visible behavior. **Exit gate:** dual-run stable in dev (live Tauri
exercise per the project's manual-test rule), no model panics.

**P3 — Switch `Attach` to serialize.** Replace the `Attach` body
(sanitize+CAN+preamble) with `serializer.Serialize(s.model)`. Land the black-box
`TestRegression_*` suite (§13.3). Live-verify in the running Tauri app: re-attach after
workspace switch (clean prompt, no `^[[?...c` garbage, correct title), vim/htop alt-screen
re-attach, daemon-restart restore. **Exit gate:** all `TestRegression_*` green + live Tauri
re-attach verified for shell + vim + htop.

**P4 — Delete the patches.** Remove `replay_sanitize.go`, `dec_modes.go`, the CAN append,
`ring.go`, `RingCap`, and their tests/`export_test` helpers. Rename `RingCap→ModelBytes`,
`maxTotalRingBytes→maxTotalModelBytes` and update `Stats`/maintenance. **Exit gate:**
build + full suite + race green; coverage gate green; `grep` confirms no remaining
ring/sanitize/dec-mode references.

**P5 — Persistence cutover.** `.buf` now holds serialized state via the unchanged
`Snapshot()`/persist paths; restore/placeholder feed serialized bytes into a fresh model.
Clear dev `.buf` + IndexedDB (no migration, NG3). Live-verify suspend→restore and
daemon-restart→restore reproduce the screen. **Exit gate:** suspend/restore + restart
restore live-verified; persistence round-trip `TestRegression_*` green.

---

## Appendix A — Concrete `pumpStep` / `Attach` diffs (illustrative)

**`Attach` (after):**

```go
func (s *Session) Attach() (<-chan OutputFrame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return nil, fmt.Errorf("session: attach: session %s is dead", s.id)
	default:
	}
	cl := &client{send: make(chan OutputFrame, clientSendBuf)}
	// Sample the foreground-reset detector FIRST (§11.1 sampling site #2), under the
	// already-held s.mu, BEFORE Serialize — load-bearing: a re-attach landing inside the
	// ≤250 ms pumpStep debounce window of a SIGKILLed app would otherwise serialize the
	// dead app's stale alt/mouse modes into the new client. This is the same normative
	// pre-Serialize sample the Snapshot()/flush/suspend/shutdown paths run.
	s.checkForegroundResetLocked()
	// Serialize the CURRENT screen into one clean ground-state redraw. No
	// sanitize, no CAN, no DEC-mode preamble — the serialized state is
	// self-contained, query-free, and fully terminated by construction.
	// serializeLocked (§8.5), NOT serializer.Serialize directly, so a Serialize/downcast
	// panic bumps modelPanics and yields nil (no redraw) instead of killing the session.
	redraw := s.serializeLocked()
	// §8.3 mid-sequence attach re-sync: append the buffered in-flight partial-sequence
	// bytes AFTER the ground-state redraw so the new client's fresh parser enters the same
	// mid-sequence state as the live clients and the next live chunk's tail completes
	// correctly (never an orphaned tail). PendingInput() is "" in ground state and is
	// appended ONLY here — never in Snapshot()/persistence (Appendix would-be-dangling .buf).
	redraw = append(redraw, s.model.PendingInput()...)
	if len(redraw) > 0 {
		cl.send <- OutputFrame{SessionID: s.id, Data: redraw}
	}
	s.clients[cl] = struct{}{}
	return cl.send, nil
}
```

**`Session` struct (after):** `ring *RingBuffer` and `decModes *decModeTracker` are
replaced by `model model.TerminalModel` and `serializer model.Serializer`, plus the
live-session cadence-flush cache fields `lastBlob []byte` and `dirty bool` (§8.4, counted in
`ModelBytes()` §9.4), the **placeholder** raw-blob field `rawBlob []byte` (§4.2/§8.4 — the
model-less placeholder's persisted blob, distinct from `lastBlob`; returned by the
`s.model == nil` `Snapshot()` fast-path), the foreground-reset sampling fields
`lastForegroundPgid int` and
`lastFgSampleAt time.Time` (§8.2 step (3) / §11.1), the `modelPanics` counter (§8.2 / §8.5), and
the `InjectLocal([]byte)` method (§12 notice injection). New methods: `serializeLocked()` and
`mutateModelLocked(func())` (the §8.5 panic-isolation wrappers every Serialize / Resize-drain /
foreground-teardown model access routes through), and `DropCachedBlob()` (the §9.4 Phase-3
pre-step cache reclaim). `Snapshot()` returns `(blob []byte, changed bool)` (§8.4); `Attach`
appends `model.PendingInput()` after the redraw for the §8.3 mid-sequence re-sync. Everything else
(lifecycle, locks, flush, suspend) is unchanged.
