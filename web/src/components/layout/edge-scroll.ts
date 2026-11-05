// Scrolls the sidebar during a drag so any row is reachable without letting go. Driven by rAF:
// an interval wastes or misses frames, and per-pointermove speed would stall a held-still hand.

/** How close to an end of the scroller starts the drag scrolling it. */
export const EDGE_SCROLL_BAND_PX = 36

/** Peak speed, in px per frame, reached at the boundary itself. */
export const EDGE_SCROLL_MAX_STEP_PX = 14

/** Ramp from a crawl at the band's inner edge to full speed at the boundary. */
function ramp(distance: number): number {
  const t = Math.min(1, Math.max(0, 1 - distance / EDGE_SCROLL_BAND_PX))
  return Math.max(1, Math.round(t * EDGE_SCROLL_MAX_STEP_PX))
}

/** Pixels to scroll this frame for a pointer at `pointerY`, given the
 *  scroller's viewport box. Negative scrolls up. */
export function edgeScrollStep(pointerY: number, top: number, height: number): number {
  if (height <= 0) return 0
  const fromTop = pointerY - top
  const fromBottom = top + height - pointerY
  if (fromTop < EDGE_SCROLL_BAND_PX) return -ramp(fromTop)
  if (fromBottom < EDGE_SCROLL_BAND_PX) return ramp(fromBottom)
  return 0
}

export interface EdgeScroller {
  /** Re-aim from a new pointer position. Pure arithmetic — no DOM access. */
  update: (pointerY: number) => void
  stop: () => void
}

export interface EdgeScrollHandlers {
  /** A frame moved the list: the row under a still pointer changed, so re-run the hit test. */
  onScrolled: () => void
  /** Loop start/stop: anything animating between the positions it invalidates must pause, or it
   *  trails the scroll by its own duration. */
  onRunningChange?: (running: boolean) => void
}

// Drives `el`'s scroll from the pointer's distance to its edges. `box` is measured once at drag
// start: the scroller neither moves nor resizes during a drag.
export function createEdgeScroller(
  el: HTMLElement,
  box: { top: number; height: number },
  handlers: EdgeScrollHandlers,
): EdgeScroller {
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
    const before = el.scrollTop
    el.scrollTop = before + step
    if (el.scrollTop !== before) handlers.onScrolled()
    frame = requestAnimationFrame(tick)
  }

  return {
    update: (pointerY: number) => {
      step = edgeScrollStep(pointerY, box.top, box.height)
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

/** The nearest ancestor that actually scrolls. Resolved once per drag from the
 *  grabbed row, so the drag needs no ref plumbed down through the tree — and it
 *  keeps working if the sidebar's scroller is ever swapped for another one. */
export function findScrollParent(from: HTMLElement | null): HTMLElement | null {
  for (let node = from?.parentElement ?? null; node; node = node.parentElement) {
    if (node.scrollHeight <= node.clientHeight) continue
    const overflowY = getComputedStyle(node).overflowY
    if (overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay') return node
  }
  return null
}
