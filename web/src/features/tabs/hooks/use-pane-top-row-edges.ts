import { useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

// See the `isAtTopEdge` check's own doc below for why this isn't 10.
const EDGE_THRESHOLD = 20

interface UsePaneTopRowEdgesResult {
  rowRef: RefObject<HTMLDivElement | null>
  isAtLeftEdge: boolean
  isAtRightEdge: boolean
  isAtTopEdge: boolean
}

/**
 * Is THIS element's own bounding box flush against a window edge? Extracted
 * from `useTabBarScroll` (the geometry it always measured off `tabBarRef`)
 * so any "pane top row" can read the same window-edge facts — TabBar's own
 * row, and now `ChatOnlyPaneHeader`'s, need the identical macOS
 * traffic-light inset (`isAtLeftEdge && isAtTopEdge`) regardless of which
 * row happens to be occupying a pane's top-left corner.
 *
 * `deps` re-runs the check for a position-only layout change a
 * `ResizeObserver` (SIZE changes only) would miss — e.g. the sidebar
 * flipping side, which shifts a pane without resizing it.
 */
export function usePaneTopRowEdges(deps: unknown[] = []): UsePaneTopRowEdgesResult {
  const rowRef = useRef<HTMLDivElement>(null)
  const [isAtLeftEdge, setIsAtLeftEdge] = useState(false)
  const [isAtRightEdge, setIsAtRightEdge] = useState(false)
  const [isAtTopEdge, setIsAtTopEdge] = useState(false)

  useLayoutEffect(() => {
    const el = rowRef.current
    if (!el) return
    function check() {
      const rect = el?.getBoundingClientRect()
      setIsAtLeftEdge((rect?.left ?? 1) < EDGE_THRESHOLD)
      setIsAtRightEdge((rect?.right ?? 0) > window.innerWidth - EDGE_THRESHOLD)
      // Top edge matters for the macOS traffic-light inset: in a vertical
      // split every pane's top row is at the window's LEFT edge, but only
      // the one at the window's TOP overlaps the window controls.
      //
      // EDGE_THRESHOLD, not 10: the pane actually touching the window's top
      // still carries its own TOP_GUTTER inset (8px) PLUS the pane box's own
      // border-top width on top of that (pane-border.ts's BORDER) — a real,
      // measured rect.top of exactly 10 with a 2px border. A `< 10` cutoff
      // missed that by a hair and silently dropped the traffic-light inset
      // (reported live: "Untitled chat" tab rendering flush under the
      // window controls). 20 leaves headroom for the border to grow again
      // without this snapping back.
      setIsAtTopEdge((rect?.top ?? 1) < EDGE_THRESHOLD)
    }
    check()
    const ro = new ResizeObserver(check)
    ro.observe(el)
    window.addEventListener('resize', check)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', check)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return { rowRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge }
}
