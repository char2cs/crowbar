# Virtualize the agent-chats sidebar list

**Date:** 2026-07-22
**Branch:** enhancement/restyling
**Status:** Design — approved direction (geometry/model-driven drag)

## Problem

The sidebar "Chats" panel (`features/agent/components/agent-chats-panel.tsx`)
renders one live DOM row per chat via `ordered.map(...)` with no windowing, and
the panel is mounted continuously inside the sidebar carousel. As the chat count
grows this costs mount time, retained DOM, and — for every busy chat — a
continuously-running SMIL spinner, even while the tab is scrolled off-screen.

A prior change (already landed) fixed the dominant symptom — a single chat's
turn frame no longer re-renders the whole list — by giving each `AgentChatRow`
its own `working[chatId]` subscription and memoising it. This spec covers the
remaining structural change: **windowing the list**, so the number of *mounted*
rows is bounded by the viewport regardless of how many chats exist.

The reason this is not a drop-in `getVirtualItems()` swap: the panel's
drag-to-reorder resolves its drop target with `document.elementsFromPoint`,
which only works because **every row is painted in the DOM**. Windowing removes
off-screen rows, so the drag mechanism must be re-architected, not patched.

## Goal / non-goals

**Goal:** Only the visible slice (+ overscan) of chat rows is mounted; all
existing behaviours (order, persisted user order, select/open, rename, active
highlight, drag-to-reorder, drag-to-trash-delete, the ghost, the New-per-provider
rows) are preserved exactly.

**Non-goals:**
- Changing reorder *semantics* ("drop before row K"; drop over nothing = no-op).
- Extracting a repo-wide shared virtualization hook (the three existing
  virtualized lists are each bespoke; a shared abstraction is deferred — noted
  as future work, not done here).
- Refactoring the file tree / git lists.
- Removing drag-to-reorder (kept; product-level "is manual reorder right at
  scale" is out of scope).

## Architecture: three separated layers

The panel today fuses model, view, and interaction. The fix separates them.

### 1. Model (unchanged)
`orderedChats(chats, order)` → the full ordered `AgentChat[]`. Pure, already
tested. Rows are a **fixed 40px pitch** (`ROW_BASE` = `h-9` 36px + `my-0.5` 4px),
so no per-row measurement is ever required. Export a shared constant
`AGENT_CHAT_ROW_HEIGHT = 40`.

### 2. View — windowed rendering
- The panel's scroll container becomes a plain `overflow-auto` div
  (`min-h-0 flex-1`), replacing base-ui `ScrollArea`. The virtualizer owns this
  element directly — matching `file-explorer-tree.tsx`, `git-history-list.tsx`,
  and `git-diff-editor-stack.tsx`, and consistent with the file tree beside it.
- `useVirtualizer` (`@tanstack/react-virtual`, already a dependency):
  `count: ordered.length`, `estimateSize: () => AGENT_CHAT_ROW_HEIGHT`,
  `getScrollElement: () => scrollRef.current`, `overscan: 8`, and the same
  pane-resize-deferred `observeElementRect` used by `useFileExplorerVisibleRows`
  (deferring ResizeObserver callbacks while `data-pane-resizing` is set, flushing
  on `pane-resize-end`) — copied locally with a comment pointing at the original.
- Render shape inside the scroll container:
  ```
  <div ref={scrollRef} class="min-h-0 flex-1 overflow-auto">
    <div style={{ height: totalSize, position: 'relative' }}>       // chat region
      {virtualItems.map(vi =>
        <div style={{ position:'absolute', inset:'0 0 auto 0',
                      transform:`translateY(${vi.start}px)`,
                      height: AGENT_CHAT_ROW_HEIGHT }}>
          <AgentChatRow ... />                                       // memoised (already)
        </div>)}
    </div>
    <Footer/>   // separator + one NewChatRow per provider — static, few, unvirtualized
  </div>
  ```
  Absolute-positioned rows (the git-diff pattern) avoid margin-collapse questions;
  the chat region's content origin is exactly row 0's box top (no leading padding
  inside the scroller — any panel padding lives on a non-scrolling wrapper) so the
  drag geometry below is clean.
- The New-per-provider rows and their hairline separator render **after** the
  `height: totalSize` block, in normal flow, so they sit below the last chat and
  scroll with the list. Pointer over that footer region resolves to no drop
  target (below the chat region) — preserving "drop over the New rows = no-op".

### 3. Interaction — drag resolved from geometry, not painted DOM

Two new **pure** functions (own module `agent-chat-drop-geometry.ts`), so drop
resolution is testable without a layout engine:

```ts
// Which chat row is the pointer over? 0..count-1, or null when above the first
// row or below the last (e.g. over the New-chat footer). Content origin = row 0.
resolveDropRowIndex(p: {
  pointerY: number          // clientY
  containerTop: number      // scrollEl.getBoundingClientRect().top
  scrollTop: number         // scrollEl.scrollTop
  rowHeight: number
  count: number
}): number | null

// Per-frame vertical scroll delta while dragging near an edge (px; <0 = up).
// 0 outside the edge zones. Lets a drag reach rows not currently painted.
autoScrollDelta(p: {
  pointerY: number
  containerTop: number
  containerHeight: number
  edge: number              // edge-zone thickness (px)
  step: number              // px per frame within the zone
}): number
```

Panel wiring (replaces `findDropTarget`/`elementsFromPoint` entirely):
- **Arm/threshold/ghost:** unchanged (`dragRef`, 5px threshold, ghost moved via
  DOM, `setDraggingId`).
- **Drop target on pointermove:**
  1. Trash first: the trash footer is always painted — hit-test it by its own
     `getBoundingClientRect().contains(x, y)` (ref to the trash element). No
     `elementsFromPoint` anywhere.
  2. Else geometry: `idx = resolveDropRowIndex(...)`; `targetId = idx == null ?
     null : ordered[idx].id`; `hoverTarget = targetId && targetId !== dragId ?
     targetId : null`. Same ring highlight as today (`dropTarget` prop).
- **Auto-scroll:** on drag-active, a rAF loop reads the latest pointer Y (a ref)
  and `autoScrollDelta`; when non-zero it scrolls `scrollRef` and **recomputes
  the hover target** (scrollTop changed even if the pointer is still). Loop starts
  when the drag goes active, stops on drag end.
- **Drop (pointerup):** resolve target once more; `onDrop` reuses the existing
  `reorderIds` (reorder) / `removeChat` (trash) — reorder semantics untouched.

`reorderIds` operates on the full id list and is index-agnostic, so it needs no
change.

## Files

**New**
- `features/agent/components/agent-chat-drop-geometry.ts` — `resolveDropRowIndex`,
  `autoScrollDelta`, and the `AGENT_CHAT_ROW_HEIGHT = 40` constant (defined here,
  imported by the panel).
- `__tests__/features/agent/components/agent-chat-drop-geometry.test.ts` — pure
  unit tests for both functions.

**Changed**
- `features/agent/components/agent-chats-panel.tsx` — swap `ScrollArea` for the
  virtualized `overflow-auto` container; render virtual rows; replace
  `findDropTarget`/`elementsFromPoint` with the geometry + trash-rect resolver +
  auto-scroll loop; footer for separator + New rows.
- `__tests__/features/agent/components/agent-chats-panel.test.tsx` — mock
  `@tanstack/react-virtual` to yield all items (the `git-diff-editor-stack.test`
  pattern) so rows render under jsdom; rewrite the ~12 drag tests to drive
  geometry (stub the scroll container's `getBoundingClientRect` + `scrollTop`;
  pointer Y → target index) instead of stubbing `elementsFromPoint`.
- `__tests__/features/agent/components/agent-chats-panel-rerender.test.tsx` — add
  the same `@tanstack/react-virtual` mock so the panel renders rows.
- Any other test that renders the *real* `AgentChatsPanel` (verify
  `sidebar-carousel.test.tsx`) gets the same virtualizer mock, via a small shared
  test helper to avoid duplication.

`AgentChatRow` is unchanged (already memoised + self-subscribed from the #2 work).

## Testing strategy

- **Pure geometry** (`agent-chat-drop-geometry.test.ts`): pointer in row 0 / mid
  / last; above list → null; below last (footer) → null; clamping at count;
  non-zero `scrollTop` offset; `autoScrollDelta` in top zone (<0), bottom zone
  (>0), middle (0), and at exact boundaries.
- **Panel integration** (`agent-chats-panel.test.tsx`): with the virtualizer
  mocked to render all rows, every existing non-drag assertion (order, persisted
  order, select/open, rename, delete, active-by-tab, New rows, empty state)
  stands. Drag tests re-expressed against stubbed container geometry.
- **Full suite + tsc + eslint + prettier + react-doctor** green (the change
  touches a shared sidebar surface).
- **Live Tauri check** (per repo convention): scroll a long chat list; confirm
  only a viewport's worth of rows are in the DOM; drag-reorder a row past the
  fold (auto-scroll) and to the trash; rename; confirm the New rows still sit
  below the list.

## Edge cases

- **Empty list:** `count = 0` → no chat region, only the New-rows footer (today's
  "empty state renders only the New rows"). `resolveDropRowIndex` → null.
- **Fewer chats than fit the viewport:** virtualizer renders them all; footer
  directly beneath; behaviour identical to today.
- **Workspace switch:** panel is keyed by `wsId` (unchanged) — virtualizer and
  drag state reset with the remount.
- **Pane resize during a drag / at rest:** the deferred `observeElementRect`
  suppresses the 120fps ResizeObserver storm, matching the file tree.
- **Drag started, pointer leaves the window / pointercancel:** existing
  `endDrag` path also stops the auto-scroll rAF loop.

## Implementation plan (TDD, phased)

1. **Geometry module (pure, RED→GREEN):** write `agent-chat-drop-geometry.test.ts`
   for `resolveDropRowIndex` + `autoScrollDelta`; implement to pass.
2. **Virtualize rendering:** add the `@tanstack/react-virtual` mock to the two
   panel test files (+ shared helper); confirm the existing non-drag panel tests
   still pass while swapping `ScrollArea` → virtualized `overflow-auto` container
   and rendering virtual rows + footer. tsc/eslint green.
3. **Geometry drag:** rewrite the drag tests to drive stubbed container geometry
   (RED against the old `elementsFromPoint` path), then replace the panel's drop
   resolver with the geometry + trash-rect + auto-scroll wiring (GREEN).
4. **Verify:** full suite, tsc, eslint, prettier, react-doctor 100, then live
   Tauri check.

Each phase leaves the suite green before the next.
