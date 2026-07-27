# Sidebar hover-peek — design

Date: 2026-07-27
Branch: `fix/misc-fixes`

## Problem

When the sidebar is hidden there is no way to glance at it. Reaching the file
tree or the chat list means toggling the sidebar open, which reflows the whole
editor, and then toggling it shut again.

## Behaviour

While the sidebar is hidden, hovering the window edge it is docked against
slides a floating copy of the sidebar in over the editor. Moving the pointer
away slides it back out. Modelled on Zen Browser's floating sidebar.

- Peeking never changes the layout. The collapsed panel stays at 0px and the
  editor does not reflow — the peek floats on top of it.
- Peeking is not pinning. Clicking inside the peeked sidebar does not open it;
  the toggle button remains the only way to dock it. The peek closes whenever
  the pointer leaves it.
- The peek is the user's remembered sidebar width, so it looks like the real
  sidebar rather than a separate mini-view.
- Mirrored when the sidebar is docked right.

## Architecture

A `SidebarPeek` wrapper is inserted between the sidebar `ResizablePanel` and
`sidebarContent` in `ide-shell.tsx`. It is always rendered, in the same
position in the tree, whether the sidebar is open, hidden, or peeking — only
its own classes change.

This is the load-bearing constraint. Rendering `sidebarContent` into a separate
overlay container when hidden would unmount and rebuild it, resetting the
workspace tree, the carousel scroll offset, the file explorer and the agent
chat list. That is the same defect class as the side-flip bug fixed earlier on
this branch (see `ide-shell.tsx`, the comment above `sidebarPanel`). Keeping
one element in one place and restyling it keeps the subtree alive.

States:

| Sidebar | Wrapper |
| --- | --- |
| open | transparent pass-through: `contents`-like, no visual or layout effect |
| hidden, not peeking | fixed peek layer, card translated fully off-screen |
| hidden, peeking | fixed peek layer, card translated to rest |

## The card

- Inset 8px from the leading window edge, the top and the bottom.
- Width = the remembered sidebar width (`preferredWidth` in `ide-shell.tsx`).
- Surface: the app's canonical floating-surface tokens (`bg-popover`, border,
  large radius, `shadow-lg/5`), matching popovers and dialogs.

The docked sidebar is deliberately fully transparent (`--sidebar` is
`oklch(0 0 0 / 0%)`) so the native window vibrancy shows through. A floating
card cannot be transparent — the editor would read through it — so the peek
uses an opaque popover surface and looks slightly different from the docked
sidebar by design.

The card spans the full window height. On macOS the traffic lights are drawn by
the OS above the webview, so they stay visible over the card, and
`SidebarProjectHeader` already reserves a 72px spacer for them on the left.

## Trigger

A hit TEST, not a hit TARGET, and no timers. While hidden, a document-level
`pointermove` listener opens the peek when the pointer is within 6px of the
docked edge, between the chrome band and the bottom corner-resize zone; it
closes when the pointer is neither there nor within the card's footprint plus
its margin. Those two regions are contiguous, so there is no gap to flicker
across and no close delay is needed — which is also the house rule (no
timer-gated UI). `pointerleave` on the document and `blur` on the window close
it, since the pointer can leave without a final move.

An element was the first design and was wrong. Anything hit-testable at the
window edge sits on top of the editor and swallows its wheel, clicks and text
selection in that column; with the sidebar docked right it would have covered
most of Monaco's 14px vertical scrollbar, making it undraggable. The card's
footprint is computed arithmetically rather than measured, so the listener
never forces layout.

The window is natively framed and resizable, so AppKit owns a resize tracking
band a few points wide at the frame. Those points are lost either way — the hit
test simply does not care, where an element would have had to be widened to
compensate.

## Animation

Transform only, so it composites: `translateX(calc(-100% - 1rem))` at rest,
`translateX(0)` when peeking, mirrored for the right side. The extra `1rem`
clears the margin and the shadow so no sliver shows while hidden. Duration and
easing follow the app's existing slide convention.

## Isolation

`sidebarOpen` stays false throughout a peek. The collapsed panel, the toggle
button's label, the persisted width and `handleSidebarResize`'s window-driven
latch are all untouched — the peek changes no panel geometry, so it emits no
resize events.

The sidebar carousel's re-align observer already handles the width going 0 →
peek width, from the fix earlier on this branch.

## Testing

Unit (jsdom, `web/src/__tests__/components/layout/`):

- The wrapper renders its children in all three states and the child DOM node
  is identical across an open → hidden → peek transition (proves no remount).
- Entering the layer while hidden peeks; leaving it un-peeks.
- While the sidebar is open, the wrapper adds no peek affordance and pointer
  events over it do nothing.
- Mirrored geometry for the right side.

Live (Tauri):

- Peek in/out on both sides, at the remembered width, with the editor not
  reflowing.
- The hover strip is reachable at the extreme window edge.
- Idle CPU with the sidebar hidden, compared against the pre-change baseline
  captured on 2026-07-27: dev webview median 13.5%, range 6.5–36.0, n=20
  samples at 2s. The card stays laid out while hidden, and this codebase has a
  history of layout-dormancy CPU regressions, so this is a gate, not a
  formality.

## Adjacent fixes this pulled in

- The parked card is `inert`, so Tab from the editor cannot walk into an
  invisible sidebar and text cannot keep landing in a search field that has
  slid away.
- `DragGhost` is portalled to the body. It is positioned in raw viewport
  coordinates but was rendered inline inside the sidebar, so the peeked card's
  transform made it the containing block: the chip drifted off the cursor and
  was clipped at the card's edge.
- `loadSidebarWidth` now clamps at both ends. The panel is bounded by its own
  min/max, but the peek card takes the number raw, so an over-large stored value
  would render it past the opposite window edge.
- The open/hidden state is persisted, so a reload restores the sidebar as it was
  left rather than always docking it.

## Not caused by this work

Hiding the sidebar drops the app from ~8ms to ~106ms frames (125fps → 9fps).
Measured on a tree with all of this branch's work stashed, so it is pre-existing
and upstream of the peek: docked 8ms median / p95 13ms, hidden 106ms median /
p95 125ms. There are zero long tasks, and forcing continuous compositing does
not recover it, so it is neither main-thread JS nor idle rAF throttling.
Untriaged.

## Out of scope

- Resizing the sidebar from the peeked state.
- Keyboard or focus-driven peeking.
- Peeking while the sidebar is docked open.
