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
- **Divergence backstop**: a debug-only (dev builds) checksum — every Nth
  emit, hash the model grid; the client (dev flag) hashes its grid post-apply
  and logs mismatches. Not shipped in release builds. This is the conformance
  canary while the flag bakes; it must stay silent for the flag to default on.

### 3.7 Rollout flag

`terminal.modelDrivenOutput` per-profile setting + `CROWBAR_TERMINAL_MODEL_DRIVEN`
env override; default ON in dev builds, OFF in release until the canary and
daily-driver period pass. Flag is read at session spawn (no mid-session
switching; a restart applies it).

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
- **Memory**: one extra cols×rows cell buffer per live session (`lastEmitted`),
  ≈ 170×50×~24B ≈ 200KB worst case; accounted in the engine memory stats.

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
