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
