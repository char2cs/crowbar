/**
 * Pure drag geometry for the virtualized agent-chats sidebar list.
 *
 * The list is windowed, so off-screen rows are not in the DOM and the drag can
 * no longer find its drop target with `document.elementsFromPoint`. Instead the
 * target — and the auto-scroll that lets a drag reach rows past the fold — are
 * derived from scroll geometry, which keeps the whole mechanism a pure function
 * (no layout engine, fully unit-testable).
 */

/** The fixed vertical pitch of one chat row: ROW_BASE `h-9` (36px) + `my-0.5`
 *  (4px). The list has no wrapping rows, so this is exact and constant — it is
 *  both the virtualizer's `estimateSize` and the drag geometry's row height. */
export const AGENT_CHAT_ROW_HEIGHT = 40

interface DropSlotParams {
  /** Pointer viewport Y (clientY). */
  pointerY: number
  /** The scroll container's top in viewport coords (getBoundingClientRect().top). */
  containerTop: number
  /** The scroll container's current scrollTop. */
  scrollTop: number
  /** Fixed row pitch (AGENT_CHAT_ROW_HEIGHT). */
  rowHeight: number
  /** Number of chat rows in the list. */
  count: number
}

/**
 * The drop SLOT under the pointer: the gap the dragged row would land in. Slot
 * `i` is the gap ABOVE row `i`, so slots run `0 … count` and slot `count` is the
 * end of the list. The content origin is row 0's box top, so the caller must not
 * put leading padding inside the scroll container.
 *
 * A SLOT, NOT A ROW, and that distinction is the whole fix. Resolving the row
 * under the pointer and inserting BEFORE it can express only `count` of the
 * `count + 1` possible destinations: there is no row after the last one, so the
 * bottom of the list was unreachable from any pointer position at all. It also
 * made the commonest drag a no-op — dropping a row onto its immediate neighbour
 * asked for "put me in front of the row I am already in front of" — while the
 * hover affordance had already promised a move.
 *
 * `Math.round` puts the boundary at each row's MIDPOINT, which is what "insert
 * above / below this row" means to a hand holding a chip, and matches the
 * insertion line the panel draws. Clamped rather than nulled: the pointer is
 * always somewhere, the line always shows where, and the drop always does what
 * the line showed.
 */
export function resolveDropSlotIndex({
  pointerY,
  containerTop,
  scrollTop,
  rowHeight,
  count,
}: DropSlotParams): number {
  const contentY = pointerY - containerTop + scrollTop
  const slot = Math.round(contentY / rowHeight)
  return Math.max(0, Math.min(slot, count))
}

interface AutoScrollParams {
  /** Pointer viewport Y (clientY). */
  pointerY: number
  /** The scroll container's top in viewport coords. */
  containerTop: number
  /** The scroll container's visible height. */
  containerHeight: number
  /** Thickness of the top/bottom edge zones that trigger scrolling (px). */
  edge: number
  /** Scroll amount per animation frame while inside a zone (px). */
  step: number
}

/**
 * The per-frame vertical scroll delta while dragging near a container edge: a
 * negative step in the top zone (scroll up), a positive step in the bottom zone
 * (scroll down), and 0 in between. A pointer beyond an edge stays in that zone,
 * so a drag held above/below the list keeps scrolling toward it.
 */
export function autoScrollDelta({
  pointerY,
  containerTop,
  containerHeight,
  edge,
  step,
}: AutoScrollParams): number {
  const distTop = pointerY - containerTop
  const distBottom = containerTop + containerHeight - pointerY
  if (distTop < edge) return -step
  if (distBottom < edge) return step
  return 0
}
