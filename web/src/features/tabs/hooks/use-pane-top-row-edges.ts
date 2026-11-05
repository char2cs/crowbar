import { useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { useHasViewStrip } from '@/features/panes/hooks/use-project-view-ids'

// The top pane's row sits at its 8px gutter plus the pane border (10px with a 2px border), so
// 20 leaves headroom. Shared with useMacTrafficLightSync, which must agree on the corner test.
export const EDGE_THRESHOLD = 20

interface UsePaneTopRowEdgesResult {
  rowRef: RefObject<HTMLDivElement | null>
  isAtLeftEdge: boolean
  isAtRightEdge: boolean
  isAtTopEdge: boolean
}

/** Whether this row's box is flush against each window edge (the traffic-light inset needs left
 *  && top). `deps` re-checks position-only changes a ResizeObserver misses, such as the sidebar
 *  flipping side; the view strip toggling is always one. */
export function usePaneTopRowEdges(deps: unknown[] = []): UsePaneTopRowEdgesResult {
  const hasViewStrip = useHasViewStrip()
  const rowRef = useRef<HTMLDivElement>(null)
  const measuredRef = useRef(false)
  const [isAtLeftEdge, setIsAtLeftEdge] = useState(false)
  const [isAtRightEdge, setIsAtRightEdge] = useState(false)
  const [isAtTopEdge, setIsAtTopEdge] = useState(false)

  useLayoutEffect(() => {
    const el = rowRef.current
    if (!el) return
    function check() {
      const rect = el?.getBoundingClientRect()
      // A parked view (display:none) has no box and reads as flush against the
      // top-left corner; once a real box was measured, keep that answer.
      const hasBox = !!rect && (rect.width > 0 || rect.height > 0)
      if (!hasBox && measuredRef.current) return
      if (hasBox) measuredRef.current = true
      setIsAtLeftEdge((rect?.left ?? 1) < EDGE_THRESHOLD)
      setIsAtRightEdge((rect?.right ?? 0) > window.innerWidth - EDGE_THRESHOLD)
      // In a vertical split every pane's row is at the LEFT edge; only the one at the TOP sits
      // under the window controls.
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
  }, [...deps, hasViewStrip])

  return { rowRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge }
}
