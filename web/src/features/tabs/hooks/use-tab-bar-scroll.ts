import { useCallback, useRef } from 'react'
import type { RefObject } from 'react'
import { usePaneTopRowEdges } from './use-pane-top-row-edges'

interface UseTabBarScrollOptions {
  sidebarPosition: string
  draggedBufferId: string | null
}

interface UseTabBarScrollResult {
  tabBarRef: RefObject<HTMLDivElement | null>
  /** The actual horizontally-scrolling element — the editor-tab scroller,
   *  since the pane-top row (`tabBarRef`) now also hosts the split toggle
   *  and the chat head, which never scroll. */
  scrollRef: RefObject<HTMLDivElement | null>
  isAtLeftEdge: boolean
  isAtRightEdge: boolean
  isAtTopEdge: boolean
  handleWheel: (e: React.WheelEvent<HTMLDivElement>) => void
}

export function useTabBarScroll({
  sidebarPosition,
  draggedBufferId,
}: UseTabBarScrollOptions): UseTabBarScrollResult {
  const {
    rowRef: tabBarRef,
    isAtLeftEdge,
    isAtRightEdge,
    isAtTopEdge,
  } = usePaneTopRowEdges([sidebarPosition])
  const scrollRef = useRef<HTMLDivElement>(null)

  const canScrollTabsHorizontally = useCallback(() => {
    const container = scrollRef.current
    if (!container) return false
    return container.scrollWidth > container.clientWidth + 1
  }, [])

  const handleWheel = useCallback(
    (e: React.WheelEvent<HTMLDivElement>) => {
      const container = scrollRef.current
      if (!container) return
      if (draggedBufferId) return
      if (e.ctrlKey || e.metaKey) return
      if (!canScrollTabsHorizontally()) return

      const hasHorizontalIntent = Math.abs(e.deltaX) > 0
      const hasShiftedVerticalIntent = e.shiftKey && Math.abs(e.deltaY) > 0
      const hasVerticalFallback = Math.abs(e.deltaX) === 0 && Math.abs(e.deltaY) > 0

      if (!hasHorizontalIntent && !hasShiftedVerticalIntent && !hasVerticalFallback) return

      const delta = hasHorizontalIntent ? e.deltaX : e.deltaY
      if (delta === 0) return

      const maxScrollLeft = container.scrollWidth - container.clientWidth
      if (maxScrollLeft <= 0) return

      const nextScrollLeft = Math.max(0, Math.min(container.scrollLeft + delta, maxScrollLeft))
      if (nextScrollLeft === container.scrollLeft) return

      e.preventDefault()
      container.scrollLeft = nextScrollLeft
    },
    [canScrollTabsHorizontally, draggedBufferId],
  )

  return { tabBarRef, scrollRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge, handleWheel }
}
