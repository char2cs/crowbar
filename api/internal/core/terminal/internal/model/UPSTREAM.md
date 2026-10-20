# `model/` backend spike findings & adoption record

Spec: `docs/superpowers/plans/2026-06-29-terminal-screen-model-engine.md` (§4.1, §6, §9.2,
§13.2, §15). This file is the spec-mandated record of the P0 spike outcomes against the
**pinned** commits, and of the **explicit adoption / deviation decisions** for the
foundational `model/` package phase. Where the spec leaves an either/or gated on a
capability, the gate's actual outcome at the pin and the decision taken are recorded here so
no spec-locked decision is made silently.

## Pinned commits (do NOT change without re-running this spike)

| module | pin |
| --- | --- |
| `github.com/charmbracelet/x/vt` | `v0.0.0-20260629091435-9c70f75e26a4` (untagged), vendored at `api/third_party/x-vt` and wired by `replace` in `api/go.mod`; not read from the module cache |
| `github.com/charmbracelet/ultraviolet` | `v0.0.0-20260622092850-f39628c8a989` |
| `github.com/charmbracelet/x/ansi` | `v0.11.7` |

## Backend adopted: `x/vt` (foundation phase)

`x/vt` is adopted for the foundational `model/` package. The `midterm` fall-through (§5.4 /
§15 exit-gate item 7) is **not** taken at this phase. Several §15 hard gates are met by `x/vt`
directly, and the soft-wrap gate (§15 item 7) by the patched copy below; the `xterm.js` oracle
(D2) is **not** met and is carried as an **explicit, recorded deviation** rather than a silent
override (see "Deviations" below). The foundational packages deliberately
do **not** yet wire into `Session`, delete the old replay path, or claim the §13.2 conformance
proof — those are P2-P5 work. The decision recorded here is therefore scoped to: *what the
foundation ships, and which gates are deferred vs. accepted, with eyes open.*

## §15 spike question outcomes (verified at the pin)

- **`vt.Callbacks` field set** — verified via `go doc x/vt.Callbacks`. Present and wired:
  `Title`, `IconName`, `AltScreen`, `CursorVisibility`, `CursorStyle`, `CursorColor`,
  `BackgroundColor`, `ForegroundColor`, `WorkingDirectory`, `EnableMode`, `DisableMode`.
  **Absent:** `Charset`, `LockingShift`, `ScrollRegion`, and any pre-scroll / line-eviction
  hook.
- **Default-color set-signal (step 2)** — MET via callbacks. `ForegroundColor` /
  `BackgroundColor` / `CursorColor` fire on app `OSC 10/11/12`, with `nil` meaning "reset to
  terminal default". The adapter sets `shadow.{fg,bg,cursorColor}Set` only on a non-nil
  callback and clears them on nil, so a never-recoloured app emits no `OSC 10/11/12` and never
  overrides the client theme (spec §4.1 contract note). The bare getters are NOT used to gate
  step 2.
- **Mode set-signal (steps 8-11)** — MET. `EnableMode`/`DisableMode` deliver `ansi.Mode`;
  the adapter records only `ansi.DECMode` private modes in the tracked set.
- **OSC 7 working-directory** — MET via the `WorkingDirectory` callback (no `parseLastOSC7`
  fallback needed).
- **Charset / SCS callback** — **ABSENT.** Fallback taken: the adapter runs an in-`Write`
  `ESC ( x` / `ESC ) x` scan into `shadow.g0/g1` (`escan.go`). **Implemented** (was the §4.1
  step-10 mandated fallback). Verified at the pin that `x/vt` resolves a **G0** line-drawing
  designation into its grid runes (`ESC ( 0` then `q` -> `─`) but does **NOT** resolve an
  **SO-invoked G1** designation (`ESC ) 0` + `SO` + `q` -> literal `q`), so re-emitting the
  designation + active shift in serialize step 10 is load-bearing for a DEC line-drawing TUI
  on the client.
- **Locking-shift (SI/SO) callback** — **ABSENT.** Fallback taken: in-`Write` `SI`(0x0F) /
  `SO`(0x0E) scan into `shadow.glLock` (`escan.go`). **Implemented.**
- **Scroll-region (DECSTBM) callback** — **ABSENT.** Fallback taken (parallel to the SCS
  scan, same mandated pattern): in-`Write` `CSI <top>;<bottom> r` scan into
  `shadow.scrollTop/Bottom/scrollRegionSet` (`escan.go`), private-marker-guarded so
  `CSI ? … r` (XTRESTORE) is not misread, and validity-guarded (`1 <= top < bottom`).
  **Implemented.**
- **Parser ground-state / buffered-partial exposure (mid-sequence attach re-sync)** — not
  needed: attach is model-driven (a serialized snapshot, then model-derived diffs), so the
  client never receives the raw continuation of a partial escape. The former in-adapter
  framing tracker (`PendingInput`) was deleted.
- **Cursor / alt-screen / visibility / shape getters** — MET (`CellAt`, `CursorPosition`,
  `IsAltScreen`, width/height) and used by the serializer through the file-local `emulator`
  seam (`vt_emu.go`).

## Deviations (recorded, NOT silent overrides)

### Patched copy of `x/vt`

`api/third_party/x-vt` is the pinned commit plus a small local patch:

- Each screen row and scrollback line carries xterm's `isWrapped` (`Emulator.RowWrapped` /
  `ScrollbackRowWrapped`): set by autowrap, moved with its row, cleared where xterm clears it, on a
  width change and on ring eviction; a row off the grid is ignored. The serializer writes a
  soft-wrapped row unbroken, so the client's xterm sets the same bit.
- ED 1 also clears the next row when it erases the cursor row whole, as xterm's comment intends;
  xterm's code (`InputHandler.ts:1218`) indexes that row without `ybase`, a bug deliberately not
  copied.
- The alternate screen keeps no scrollback, and DECSTBM and DECSLRM clamp their margins to the
  screen, as xterm does.

Delete the copy (and the `replace` in `api/go.mod`) once a tagged upstream has every behaviour
listed above.

### D1a — parse-panic recovery discards scrollback CONTENT **ACCEPTED RESIDUAL**

Scrollback is sourced from `x/vt`'s own buffer rather than an adapter-owned compact ring (`x/vt`
has no pre-scroll eviction hook), so the §4.1/§8.2 parse-panic recovery (`vtModel.Write` →
`recreateEmu` → `buildEmu`) builds a **brand-new blank emulator**, which discards **all
scrollback CONTENT (history)** along with the visible grid. Only the configured scrollback
**depth** (the `SetScrollbackSize` capacity, re-applied by `buildEmu`) and the title (shadow
state, not parser state) survive the recreate.

The spec §13.1 panic-recovery wording ("the compact scrollback ring + `sbBytes` are preserved
(history not lost)") presumes the §9.2 compact-ring mechanism this foundation defers.
With no adapter-owned ring at this pin, there is nothing to preserve history in across the
emulator swap, so the spec's "history not lost" guarantee is **not** met on the degraded path.

**Decision for the foundation phase:** accept the loss as a self-healing degraded-path corner
rather than reintroduce the deferred compact ring solely to survive a rare recovered panic. The
recreate is reached only after `x/vt` (an untagged emulator) panics on arbitrary PTY bytes; the
running app repaints on its next frame and live clients already received the raw bytes verbatim,
so the only exposure is a client that attaches in the window between the panic and the next
repaint, which then sees a blank screen with no history. The previously-misleading `Write` doc
comment ("scrollback depth and title survive") has been corrected to state the content loss
explicitly.

- Asserted as a tracked, non-regressing fact by `TestWriteRecoverDiscardsScrollbackContent`.
- Documented in the `vtModel.Write` doc comment.
- **Revisit trigger:** the moment the compact-ring / `sbBytes` mechanism is adopted (it needs a
  pre-scroll eviction hook, which the pinned `x/vt` lacks), the ring becomes adapter-owned and
  outlives the emulator, so `recreateEmu` preserves it and this residual is deleted with its test.

### D2 — `xterm.js` `SerializeAddon` buffer-equivalence oracle (§6.1 / §13.2 / §15 P1) **DEFERRED**

The spec designates `xterm.js`'s `SerializeAddon` as THE conformance oracle and requires proving
our payload reproduces the model's buffer in a fresh `xterm.js` across real-app PTY fixtures
(pinned `@xterm/xterm` + `@xterm/addon-serialize`). That harness is a Node/JS deliverable; this
workflow's gate is `go test` over the foundational Go packages only, and per §15 the conformance
harness is a **P1 exit-gate** item (and the real-app PTY fixtures, DECSTR-autowrap /
region-relative-CUP / soft-wrap behaviors it checks, are most meaningful once `Attach` actually
serializes — P3).

**Decision for the foundation phase:** the foundation does **not** claim the mandated
conformance proof. The in-package `TestRoundTripThroughXVT` is an `x/vt` -> serialize -> fresh
`x/vt` **self-fixpoint**: it compares every cell + cursor + restored modes against a fresh
emulator and catches encoder regressions, but it is structurally incapable of proving
equivalence to the actual `xterm.js` client. Its own header says so; it must **not** be
presented as the oracle. The `xterm.js` `SerializeAddon` harness is tracked as a deferred
P1/P3 deliverable.

Known `x/vt`-vs-`xterm.js` divergences the self-fixpoint cannot see (found with the wrap bit, not
patched):

- A wide glyph that exactly fills the last two columns leaves no pending wrap in `x/vt` (`utf8.go`
  sets `atPhantom` only from the last column), so the next character overwrites its second half;
  `xterm.js` wraps that character onto the next row.
- A wide glyph that does not fit the last column is dropped by `x/vt`; `xterm.js` wraps it early
  and leaves the last cell empty.
- EL 0 at a pending wrap erases the last cell and cancels the wrap in `x/vt`; `xterm.js` erases
  nothing and keeps the wrap. The serializer never sends EL 0 there.

## Blocking exit checklist before this package is wired or shipped

Both deviations above are accepted **only** for the not-yet-wired foundation phase. To stop
either gate-override from silently becoming the permanent contract, the forward obligations are
consolidated here as a single greppable gate list. Each line is a hard precondition; the work
that trips it (the §P3 `Session` `Attach` wiring that first calls `Serialize` on a live client,
and the production enablement of that path) MUST NOT land until its box is checked.

- [ ] **D1a — panic-recovery scrollback-content loss.** Once an adapter-owned compact ring exists
  (it needs a pre-scroll eviction hook), `recreateEmu` must preserve it so history survives the
  emulator swap; delete `TestWriteRecoverDiscardsScrollbackContent`.
- [ ] **D2 — `xterm.js` `SerializeAddon` buffer-equivalence oracle — land BEFORE the
  serialize-on-attach path is enabled in production.** Commit the Node harness with a pinned
  `@xterm/xterm` + `@xterm/addon-serialize` lockfile and the §13.2 real-app PTY fixtures (vim,
  htop, less, softwrap, linedraw, widewrap, oscolor, hyperlink, autowrap-off, CJK), and make it
  pass. The in-package `TestRoundTripThroughXVT` self-fixpoint is NOT a substitute and must not be
  presented as the oracle.

Until every box is checked, `Serialize` output is foundation-only: correct cells, wrap bits,
cursor and modes, with the D2 unproven-against-`xterm.js` caveat in force.

## Mechanism constants locked

- `vtScrollbackWindow` / x/vt retained window: not applicable at this pin — `x/vt` owns its
  built-in scrollback and the eviction hook is absent, so the foundation sources scrollback from
  `x/vt`'s own buffer (`Scrollback().Line(y)`) rather than the `WithScrollback(0)` +
  compact-`lineRing` mechanism (which depends on the absent hook). The compact-ring / `sbBytes`
  accounting (§9.2/§9.4) is therefore also deferred, to D1a's revisit trigger.
- Step-2 set-signal: `Callbacks.{Foreground,Background,Cursor}Color` (recorded above).
- Locking-shift / charset / scroll-region: in-`Write` scan (`escan.go`), recorded above.

## Diff emitter backend (2026-07-03, model-driven rendering P0)

Hand-rolled line diff adopted (diff.go) over ultraviolet's renderer:
uv's diff/render pipeline is a TUI *presenter* (owns cursor state, assumes it
is the only writer, renders to an io.Writer with its own frame lifecycle);
constraining its output to our emit contract (absolute CUP per dirty row, pen
reset per row via encodeLine, no cursor-state ownership) would mean fighting
the library. The hand-rolled diff is ~120 lines and reuses the serializer's
encodeGridRow/encodeLine, so both emission paths share one cell renderer.
