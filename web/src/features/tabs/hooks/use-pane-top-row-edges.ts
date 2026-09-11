import { useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

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
      setIsAtLeftEdge((rect?.left ?? 1) < 10)
      setIsAtRightEdge((rect?.right ?? 0) > window.innerWidth - 10)
      // Top edge matters for the macOS traffic-light inset: in a vertical
      // split every pane's top row is at the window's LEFT edge, but only
      // the one at the window's TOP overlaps the window controls.
      setIsAtTopEdge((rect?.top ?? 1) < 10)
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
