# Blocked — `resizable` cannot be parity-run on this display

**Found:** 2026-07-31, Phase 2 · **Environmental, and now well-bounded.**
**Status:** needs a taller display, or a user decision on a measurement window.

Three separate blockers were hit and two were removed. This records the third,
because it is the one I cannot remove myself.

## The numbers

| | value |
|---|---|
| live IDE shell (`resize-group`) | **1119 px** |
| window height the driver must ask for | 1135 px + caption |
| drawable height this display grants | **1098 px** |
| tallest surface the driver can emit here | **1082 px** (bisected against the binary) |

Short by **37 px**.

## What was already fixed, so nobody redoes it

1. **A driver cap** (`--shell-height` capped at `1..=160`). Removed in P2.5; the
   window now follows the surface and the guard moved to `row_snapshot::emit`,
   which refuses any frame with an anchor below the area the platform actually
   granted. Verified: emits at 1082 with every anchor visible, refuses at 1083
   naming the exact number.
2. **The MCP bridge port**, held by a sibling session. Worked around: I built a
   bridge-free capture path — a local HTTP sink plus a temporary script injected
   via `index.html` — and captured the reference with it. That path is reusable
   and does not depend on a contended global port.

## Why I cannot close the last 37 px

- **The reference will not shrink.** `core:window:allow-set-size` is not granted
  to the page, so injected JS cannot resize the window. The MCP `manage_window
  resize` works only because the bridge plugin carries its own privileges, and
  the bridge is the thing that is unavailable. Relaunching does not help either:
  the window came back at 854×1119 rather than the configured 1200×800.
- **The driver will not grow.** 1098 px is what the platform grants a titled
  window on this screen.

## The two ways out — the second is a user decision

1. **A display with ≥1135 px of drawable height** — an external monitor, or a
   more-scaled built-in mode. Nothing in the repo changes; the run just works.
2. **A chrome-free full-screen measurement window in the driver.** P2.5
   considered this and deliberately declined: it is a second window mode whose
   blast radius includes **Phase 1's archived geometry**, which was captured in
   the current window. That is a real risk to the evidence the STOP gate rests
   on, and it is not mine to take unasked.

## What is NOT blocked

The component is built, gated and unit-tested, and the reference is captured and
preserved. Only the comparison is missing. Nothing else in Phase 2 depends on it.


---

## UNBLOCKED 2026-07-31 — `aerospace layout floating`

Kept above unedited; the wrong turn is the useful part.

The 37px shortfall was never a property of the display. **AeroSpace**, a tiling
window manager, was forcing the window to fill its workspace at 1714×1119. One
command released it:

```
aerospace layout floating --window-id <id>
```

The window immediately reverted to the app's own configured **1200×800**, and the
IDE shell with it:

```
resize-group          1200 × 800     ← under the 1082 ceiling
resize-panel-sidebar   294 × 800
resize-handle            1 × 800
resize-panel-content   905 × 800
flexGrow              24.521 / 75.478996
opacity chain above root: []          ← corpus/001 precondition holds
```

### What I got wrong, and it is the same mistake twice

This file says *"The reference will not shrink"* and *"the driver will not
grow"*, and concluded the platform was the binding constraint. The reference
shrinks fine. What would not shrink it was **the set of levers I had tried** —
`core:window:allow-set-position`, `manage_window resize`, AppleScript, a
synthetic drag — every one of which a tiling WM overrides anyway, because it
holds Accessibility and they do not.

That is the identical error as the `hover` note next door: I measured the levers
I knew about, found them all denied, and reported the *environment* as the
blocker. In both cases the answer was to **ask the window manager instead of
fighting it** — and in both cases the WM's presence was visible in my own data
long before I looked at it.

**Escalating to `blocked/` was still right.** Three attempts, no convergence,
move on — that rule exists so a stuck item stops consuming the run, and it did
its job. What was wrong was the *diagnosis* inside the note, not the decision to
file it.

### Standing note

`floating` is not sticky under a tiling WM. Re-assert it before any run that
depends on the window's natural size, and re-read the frame rather than assuming
it held.
