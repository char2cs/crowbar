# Model-Driven Terminal Rendering — Design

Date: 2026-07-03
Status: draft for review
Depends on: PR #26 (snapshot frames, `{type:"resync"}`, whole-frame Rust forwarding)
Prior art: `docs/superpowers/plans/2026-06-29-terminal-screen-model-engine.md`,
`model/UPSTREAM.md` (x/vt pin capabilities)

## 1. Motivation

Today the client's xterm.js buffer is built by parsing **raw PTY bytes** on its
own, in parallel with the daemon's VT screen model. Two interpreters over one
byte stream means the client copy can diverge — and every terminal bug family
we have shipped fixes for lives in that gap: resize reflow junk, silent
transports, blank-until-scroll, replay re-triggering, split-rune corruption.
PR #26 made the model authoritative **at boundaries** (attach, resize) via
snapshot frames. This design makes it authoritative **continuously**: the
client never receives a byte the model did not derive, so client state ≡ model
state at every frame, by construction.

xterm.js is retained as the renderer (WebGL canvas, selection, links, IME,
scrollbar — all battle-tested on WKWebView). Only its role as an independent
*state owner* is removed: it still parses ANSI, but the ANSI it sees is a
projection of the model, not the raw PTY stream.

## 2. Goals / non-goals

Goals:

- Client buffer provably converges to the model every frame; divergence bugs
  become structurally impossible rather than individually patched.
- No regression in: echo latency (≤ one frame-clock tick worse, target ≤ 8ms
  added), fast-scroll throughput (`cat` a large file), full-screen TUI
  smoothness (Claude Code), scrollback UX (selection/search/scroll untouched).
- Graceful degradation: per-session opt-out flag AND automatic fallback to the
  current raw-stream path when the model is degraded (sticky parse-panic
  state) — the raw path is not deleted in this phase.

Non-goals (explicitly out of scope):

- Replacing xterm.js or its renderer.
- Structured (non-ANSI) diff wire format, client-side cell grids, scrollback
  paging-on-demand. The wire stays ANSI-in-`data`; only its provenance changes.
- Deleting the raw-stream code path (that is a later cleanup once the flag has
  baked).

## 3. Architecture

### 3.1 Emission pipeline (per session, daemon-side)

Today (`session.pumpStep`, §11.1 ordering):

    PTY chunk → fanOutLocked(raw chunk) → model.Write(chunk) → fg-sample

Model-driven:

    PTY chunk → model.Write(chunk) → markPendingEmit() → fg-sample
                                          │
    frame clock (adaptive, §3.3) ─────────┴→ emitFrameLocked():
        1. scrollback delta   (lines committed to model scrollback since last emit)
        2. screen diff        (model grid vs. lastEmitted grid → minimal ANSI)
        3. cursor/mode delta  (shadow state vs. lastEmitted shadow)
        → fanOutLocked(frame bytes)          [ordinary, non-snapshot frame]

The model write already runs synchronously inside the same critical section
today, so the reordering adds no lock traffic. The §11.1 foreground-sample
stays last.

### 3.2 Diff computation

The pinned x/vt exposes **no damage callbacks** (see UPSTREAM.md), so damage
is computed by **grid comparison at emit time**, not tracked per-write:

- The session keeps a `lastEmitted` cell buffer (cols×rows) + cursor/mode
  shadow copy, updated on every emit and replaced wholesale on every keyframe.
- Screen diff: compare model grid to `lastEmitted` line by line; for each
  dirty line emit `CUP(row,1)` + the re-rendered line (the serializer's
  existing per-line rendering, extracted for reuse). Runs of unchanged lines
  emit nothing.
- **Spike (plan phase, P0)**: whether `ultraviolet`'s buffer-diff renderer (a
  pinned dependency already used by the serializer) can produce this ANSI
  directly. If yes, adopt it; if its output can't be constrained to our
  contract, hand-roll the line diff (the serializer already owns per-line
  rendering — this is small).
- Scrollback delta: `emu` scrollback length is monotonic per screen-buffer
  epoch; the session records `lastEmittedScrollbackLen`. New lines are emitted
  FIRST, as: `CUP(rows,1)` + line + `\n` per committed line (the client
  scrolls them into its own scrollback naturally, same technique as the
  serializer's scrollback flow). Then the screen diff repaints. Alt screen has
  no scrollback: skip step 1, diff the alt grid.
- Resize/clear-screen/alt-screen-switch invalidate `lastEmitted` → the next
  emission is a **keyframe** (snapshot frame via the existing serializer)
  instead of a diff. Cheap, correct, and already client-supported.

### 3.3 Frame clock (latency vs. throughput)

Adaptive emit, tuned for the two regimes:

- **Echo regime** (interactive typing): if no emit happened in the last
  `minEmitInterval` (8ms), emit immediately on write — added echo latency is
  the diff cost of a near-empty delta (microseconds).
- **Burst regime** (`cat`, TUI repaint storms): writes arriving within 8ms of
  the last emit only mark pending; a timer fires the next emit at the 8ms
  boundary. Worst case the client receives 125 frames/s of screen-sized diffs;
  at 170×50 that is ≈ 85KB/frame ≈ 10MB/s over a unix socket — measured
  against today's raw path in the P0 bench before adoption.
- The existing writePump coalescing stays as a second-layer backstop.

### 3.4 Frame taxonomy (wire, unchanged from PR #26)

- `snapshot:true` — full ground-state redraw; client resets buffer first.
  Emitted on: attach, resize resync, `lastEmitted` invalidation (§3.2),
  divergence backstop (§3.6). Coalescing barrier in writePump (shipped).
- `snapshot` absent — incremental bytes; with the flag ON these are
  model-derived diff frames, with it OFF they are raw PTY bytes. **The client
  cannot and need not distinguish** — that is the point of keeping ANSI.

### 3.5 Client changes

Nearly none. xterm consumes frames as today. Two adjustments:

- The client no longer needs its own scrollback cap to exceed the model's
  (they now describe the same history); config stays as-is, documented.
- The PR #26 resize-resync request stays (it now returns a keyframe under the
  flag — same behavior, one mechanism).

### 3.6 Failure & recovery

- **Model degraded / parse panic** (`modelPanics` sticky state): the session
  flips to raw-stream emission for its remaining lifetime and logs once. The
  existing §8/§9.4 degraded machinery is the trigger; no new states.
- **Slow client**: unchanged — overflow disconnects the client (existing
  fanOutLocked behavior); re-attach delivers a keyframe. Diff frames are
  therefore reliable-or-reattach; no per-client diff state exists.
- **Divergence backstop** (as shipped): an env-gated, daemon-side **shadow
  client simulator**, not a client-side grid hash. When
  `CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY=1` is set, a model-driven session builds a second
  `TerminalModel` (`canarySim`) and mirrors every fanned-out frame into it —
  applying keyframes as a fresh-model reset and diffs as an incremental write,
  exactly as a real client would. After each frame it compares
  `model.GridHash(authoritative)` against `GridHash(canarySim)` and counts +
  throttled-logs mismatches (`CanaryDivergences()`). This runs in ANY build (it
  is env-gated, not build-tagged), needs no client cooperation, and is a pure
  observer: a panic in its body disables the canary for that session only,
  never the authoritative path. It is the conformance canary while the flag
  bakes; it must stay silent (zero divergences) for the flag to default on.

### 3.7 Rollout flag

`CROWBAR_TERMINAL_MODEL_DRIVEN` env override + build default (ON in dev, OFF in
release until the canary and daily-driver period pass). Flag is read at session
spawn (no mid-session switching; a restart applies it).

**Deferred**: the `terminal.modelDrivenOutput` per-profile setting is a
follow-up — only the env override and the build default shipped in this phase.
The env+build gate is sufficient for the bake period; the per-profile UI toggle
is added once the flag is ready to be user-visible.

## 3.8 Device queries (amendment, plan phase)

In raw mode the client xterm answers device queries (CPR `ESC[6n`, DA,
OSC 10/11/12 color queries) by writing replies into the PTY. Model-driven
clients never receive the queries, so the daemon answers them from the model:
x/vt already synthesizes the replies into its response pipe (drained and
discarded today). Under the flag the session installs a response sink that
writes those bytes to the PTY master. Flag off → sink nil → discarded as
today (the client remains the answerer). One answerer at a time, always.

## 4. Performance analysis

- **Echo latency**: +diff-of-tiny-delta on the emit path (µs); the immediate
  path in §3.3 keeps single-keystroke echo un-batched. Budget: no measurable
  regression in the P0 bench (keystroke → frame-out, daemon-side).
- **Fast scroll**: scrollback-delta emission is O(new lines) — same as raw.
  Screen diff per tick is O(rows) compares + O(dirty) renders. `cat` becomes
  scrollback-append + a final screen diff per tick — comparable to raw
  streaming, minus xterm parsing bytes it will immediately scroll away
  (model-driven can actually send LESS under extreme burst).
- **TUI repaint** (Claude Code): frames are bounded by the 8ms clock; a full
  repaint tick is one screen of ANSI (≈85KB worst case) — today's raw path
  already ships similar volume through the same pumps.
- **Memory**: one extra cols×rows cell buffer per live session (the diff
  emitter's `lastGrid`), ≈ 170×50×~32B ≈ 270KB worst case. `Session.ModelBytes()`
  now folds `DiffEmitter.EstimatedBytes()` (a coarse cols×rows×32 estimate of
  that grid) into the engine memory ceiling, plus — when the env-gated canary is
  running — the shadow sim's own `ModelBytes()` (it is a second full model, so
  it roughly doubles a session's footprint while enabled; acceptable for a
  dev/observer path).

## 5. Testing

- **Conformance (the centerpiece)**: property test — feed recorded + generated
  byte corpora (the §13.2 corpus, TUI captures, adversarial splits) through a
  session with the flag ON; apply the emitted frame stream to a second x/vt
  instance ("client simulator"); assert grid+cursor+modes equality with the
  session model after every frame. This is the "by construction" proof as a
  test.
- **Regime benches**: echo latency, `yes`/`cat` throughput, TUI repaint volume
  — flag ON vs OFF, gate on no-regression thresholds.
- **Unit**: scrollback-delta bookkeeping across epochs (clear, alt-screen,
  resize), lastEmitted invalidation → keyframe, degraded fallback flip,
  adaptive clock (immediate vs batched paths).
- **Existing suites must stay green with the flag OFF unchanged**, proving the
  raw path is untouched.
- **Live protocol** (manual, in the running Tauri app — never claim done from
  tests alone): Claude Code under resize storms, reload, workspace switches,
  monitor moves — the full bug-family checklist from the 2026-07-03 session.

## 6. Implementation phases (preview for the plan)

- P0: spikes — ultraviolet diff API fitness; bench harness for the three
  regimes on the raw path (baseline numbers).
- P1: `lastEmitted` buffer + line-diff emitter + scrollback delta (pure model/
  package units, conformance test first).
- P2: session pump reordering + adaptive frame clock + flag + degraded
  fallback.
- P3: dev-build divergence canary; live verification pass; bake period.
- P4 (separate, later): delete raw path once the flag has defaulted ON through
  a release cycle.

## 7. Open questions (resolved decisions)

- Ultraviolet vs hand-rolled diff → P0 spike decides; both are acceptable.
- Mid-session flag switching → rejected (restart applies); avoids a hybrid
  state machine for negligible benefit.
- Structured diff wire format → rejected for this phase (ANSI keeps xterm and
  the browser transport unchanged); revisit only if profiling shows ANSI
  re-parse cost matters.

## 8. Post-implementation deviations

Recorded during the final whole-branch review (2026-07-03); the sections above
are amended in place, this subsection is the change log.

- **Divergence canary (§3.6)**: shipped as an env-gated (`CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY=1`)
  daemon-side shadow client-simulator that mirrors fanned-out frames and compares
  grid hashes, NOT the originally-described client-side post-apply hash. It works
  in any build and needs no client cooperation.
- **Rollout flag (§3.7)**: only the env override + build default shipped; the
  `terminal.modelDrivenOutput` per-profile setting is deferred to a follow-up.
- **Scrollback-ring rotation (§3.2 scrollback delta)**: x/vt's scrollback ring
  evicts the oldest line at cap, so `ScrollbackLen()` plateaus once saturated and
  a plain `sbLen > lastLen` compare stops seeing scrolled-off lines. The diff
  emitter now anchors on the FNV-1a hash of the last scrollback line and scans
  backward (bounded by `rotationScanLimit=256`) to recover the true new-line
  boundary across rotation; an anchor evicted past the scan window forces a
  keyframe. One mechanism covers both plain growth and growth-with-rotation.
- **Active scroll region / origin mode (§3.2)**: a DECSTBM region or DECOM
  (mode 6) set BEFORE the diff base was primed passes the change-guard, but the
  client's park-at-`CUP(rows,1)`+LF scrollback trick is confined to the region
  and cannot deposit committed lines into client history. When committed
  scrollback lines coincide with an active region/origin, the emitter now forces
  a keyframe (reset clears + re-asserts) rather than emitting a diff.
- **Memory accounting (§4)**: `Session.ModelBytes()` now includes the diff
  emitter's `lastGrid` estimate and the canary sim's model bytes.
- **Unsaturated-ring boundary scan (§3.2 scrollback delta)**: the rotation scan
  above ran on EVERY scrollback growth, including below saturation, where the
  previous length is already the exact boundary and the scan could only
  misanchor on a newer line whose content happens to duplicate the anchor
  (e.g. a freshly committed batch ending in a blank line) — silently dropping
  the whole batch from client history. Fixed by scanning only once the ring
  has saturated (`sbLen >= scrollbackLines`); below that, the emitter returns
  the previous length directly.
- **Residual: identical line at the saturation boundary** — with the scan now
  confined to the saturated ring, a run of content-identical scrollback lines
  spanning the saturation transition can still misanchor the scan onto a
  newer duplicate, misplacing the new-line boundary by the run's length.
  Accepted-by-argument (rare, self-heals on the next keyframe); not exercised
  by the bake gate — see the canary-blindness note below.
- **Grid-hash canary is blind to scrollback residuals**: the divergence canary
  (§3.6) compares `GridHash`, which covers only the visible grid, cursor, and
  alt-screen flag — it never reads scrollback content. A misplaced
  scrollback boundary (either residual above) produces zero canary signal;
  "canary silent" cannot be used as the bake's clearance criterion for these,
  which is why they are accepted by argument rather than bake evidence.

  Bake-watch notes (things to keep an eye on, not blockers):
  - Keyframe-per-tick cadence cliff when a single tick lands more than
    `rotationScanLimit=256` distinct scrollback lines on an already-saturated
    ring — the scan gives up and forces a keyframe every such tick.
  - Same cliff when apps scroll heavily inside an active DECSTBM region (the
    Finding B keyframe guard fires per qualifying emit, not once).
  - `BenchmarkDiffScrollBurst` writes identical lines each iteration, so its
    post-fix number under-measures the delta path's real cost (the
    saturated-path anchor scan is cheaper against repeated content than
    against distinct content); re-baseline with distinct lines later.
  - A rare pre-Prime DECOM positioned-write residual is grid-level, not
    scrollback-level, so it IS canary-visible (unlike the two residuals
    above).
