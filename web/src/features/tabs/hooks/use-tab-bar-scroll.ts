import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

interface UseTabBarScrollOptions {
  sidebarPosition: string
  draggedBufferId: string | null
}

interface UseTabBarScrollResult {
  tabBarRef: RefObject<HTMLDivElement | null>
  isAtLeftEdge: boolean
  isAtRightEdge: boolean
  isAtTopEdge: boolean
  handleWheel: (e: React.WheelEvent<HTMLDivElement>) => void
}

export function useTabBarScroll({
  sidebarPosition,
  draggedBufferId,
}: UseTabBarScrollOptions): UseTabBarScrollResult {
  const tabBarRef = useRef<HTMLDivElement>(null)
  const [isAtLeftEdge, setIsAtLeftEdge] = useState(false)
  const [isAtRightEdge, setIsAtRightEdge] = useState(false)
  const [isAtTopEdge, setIsAtTopEdge] = useState(false)

  useLayoutEffect(() => {
    const el = tabBarRef.current
    if (!el) return
    function check() {
      const rect = el?.getBoundingClientRect()
      setIsAtLeftEdge((rect?.left ?? 1) < 10)
      setIsAtRightEdge((rect?.right ?? 0) > window.innerWidth - 10)
      // Top edge matters for the macOS traffic-light inset: in a vertical
      // split every pane's tab bar is at the window's LEFT edge, but only the
      // one at the window's TOP overlaps the window controls.
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
  }, [sidebarPosition])

  const canScrollTabsHorizontally = useCallback(() => {
    const container = tabBarRef.current
    if (!container) return false
    return container.scrollWidth > container.clientWidth + 1
  }, [])

  const handleWheel = useCallback(
    (e: React.WheelEvent<HTMLDivElement>) => {
      const container = tabBarRef.current
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

  return { tabBarRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge, handleWheel }
}
