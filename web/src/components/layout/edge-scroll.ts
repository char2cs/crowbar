/**
 * Scrolling the sidebar while a drag is in flight.
 *
 * Without it a drag from the last project to the first is impossible without
 * letting go — the sidebar is one scroller now, so the reachable target set is
 * whatever happens to be painted.
 *
 * The loop is a `requestAnimationFrame`, which is the only clock that matches
 * what it drives: an interval either fires between frames (a wasted write) or
 * misses one (a visible stutter), and scrolling per pointermove ties the speed
 * to how fast the user's hand happens to be moving, which is exactly backwards
 * for reaching something they are holding still over.
 *
 * Restored verbatim (Task 21) from before the workspace tree/chats panel
 * retirement (commit f119a402) — generic, tree-agnostic, and exactly what one
 * unified drag arm needs; only its two former consumers were tree-specific.
 */

/** How close to an end of the scroller starts the drag scrolling it. */
export const EDGE_SCROLL_BAND_PX = 36

/** Peak speed, in px per frame, reached at the boundary itself. */
export const EDGE_SCROLL_MAX_STEP_PX = 14

/** Ramp from a crawl at the band's inner edge to full speed at the boundary. */
function ramp(distance: number): number {
  const t = Math.min(1, Math.max(0, 1 - distance / EDGE_SCROLL_BAND_PX))
  return Math.max(1, Math.round(t * EDGE_SCROLL_MAX_STEP_PX))
}

/**
 * Pixels to scroll this frame for a pointer at `pointer` along the scroll axis, given the
 * scroller's viewport extent (`top`/`height` on y, left/width on x). Negative scrolls back.
 */
export function edgeScrollStep(pointer: number, top: number, height: number): number {
  if (height <= 0) return 0
  const fromTop = pointer - top
  const fromBottom = top + height - pointer
  if (fromTop < EDGE_SCROLL_BAND_PX) return -ramp(fromTop)
  if (fromBottom < EDGE_SCROLL_BAND_PX) return ramp(fromBottom)
  return 0
}

export interface EdgeScroller {
  /** Re-aim from a new pointer position along the axis. Pure arithmetic — no DOM access. */
  update: (pointer: number) => void
  stop: () => void
}

export interface EdgeScrollHandlers {
  /**
   * A frame that actually moved the list. The row under a held-still pointer
   * changes as the list slides past it, so the hit test has to run again.
   */
  onScrolled: () => void
  /**
   * The loop starting and stopping. Anything animating between the positions
   * this loop invalidates has to stop animating while it runs, or it trails the
   * scroll by its own duration instead of tracking it.
   */
  onRunningChange?: (running: boolean) => void
}

/**
 * Drive `el`'s scroll from the pointer's distance to its edges.
 *
 * `box` is read ONCE by the caller at drag start and cached here: the scroller
 * neither moves nor resizes during a drag, so measuring it per pointermove
 * would be a forced layout for an answer that cannot have changed.
 */
export function createEdgeScroller(
  el: HTMLElement,
  box: { top: number; height: number } | { left: number; width: number },
  handlers: EdgeScrollHandlers,
): EdgeScroller {
  // A box with a `left` is a horizontal scroller (the view tab strip): scrollLeft off the pointer's x.
  const horizontal = 'left' in box
  const start = horizontal ? box.left : box.top
  const size = horizontal ? box.width : box.height
  const read = () => (horizontal ? el.scrollLeft : el.scrollTop)
  const write = (v: number) => {
    if (horizontal) el.scrollLeft = v
    else el.scrollTop = v
  }
  let frame: number | null = null
  let step = 0

  /** Release the frame and say so, once. */
  const halt = () => {
    if (frame === null) return
    cancelAnimationFrame(frame)
    frame = null
    handlers.onRunningChange?.(false)
  }

  const tick = () => {
    if (step === 0) {
      halt()
      return
    }
    const before = read()
    write(before + step)
    if (read() !== before) handlers.onScrolled()
    frame = requestAnimationFrame(tick)
  }

  return {
    update: (pointer: number) => {
      step = edgeScrollStep(pointer, start, size)
      if (step === 0 || frame !== null) return
      handlers.onRunningChange?.(true)
      frame = requestAnimationFrame(tick)
    },
    stop: () => {
      step = 0
      halt()
    },
  }
}

/**
 * The nearest ancestor that actually scrolls. Resolved once per drag from the
 * grabbed row, so the drag needs no ref plumbed down through the tree — and it
 * keeps working if the sidebar's scroller is ever swapped for another one.
 */
export function findScrollParent(from: HTMLElement | null): HTMLElement | null {
  for (let node = from?.parentElement ?? null; node; node = node.parentElement) {
    if (node.scrollHeight <= node.clientHeight) continue
    const overflowY = getComputedStyle(node).overflowY
    if (overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay') return node
  }
  return null
}
