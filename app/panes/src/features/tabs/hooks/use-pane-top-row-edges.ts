import { useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { useHasViewStrip } from '@/features/panes/hooks/use-tab-view-ids'

const EDGE_THRESHOLD = 20

interface UsePaneTopRowEdgesResult {
  rowRef: RefObject<HTMLDivElement | null>
  isAtLeftEdge: boolean
  isAtRightEdge: boolean
  isAtTopEdge: boolean
}

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
      const hasBox = !!rect && (rect.width > 0 || rect.height > 0)
      if (!hasBox && measuredRef.current) return
      if (hasBox) measuredRef.current = true
      setIsAtLeftEdge((rect?.left ?? 1) < EDGE_THRESHOLD)
      setIsAtRightEdge((rect?.right ?? 0) > window.innerWidth - EDGE_THRESHOLD)
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
