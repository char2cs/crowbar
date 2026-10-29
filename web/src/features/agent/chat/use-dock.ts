import { useCallback, useEffect, useRef, useState } from 'react'
import { layoutHeight } from '@/lib/layout-size'
import { playArrival } from '@/features/agent/chat/lib/arrival-animation'
import { measureScrollbarWidth } from '@/features/agent/chat/lib/scrollbar-width'

/**
 * The dock's height and the scrollbar's width, published to CSS: the transcript
 * runs under the dock and must reserve exactly its room, and the dissolve stops
 * at the scrollbar track. A first send slides the dock in from the empty document.
 */
export function useDock() {
  const [dockHeight, setDockHeight] = useState(0)
  const [scrollbarWidth, setScrollbarWidth] = useState(0)
  useEffect(() => setScrollbarWidth(measureScrollbarWidth()), [])
  const observer = useRef<ResizeObserver | null>(null)
  const arrivalOrigin = useRef<DOMRect | null>(null)
  const dockRef = useCallback((node: HTMLDivElement | null) => {
    observer.current?.disconnect()
    if (!node) {
      setDockHeight(0)
      return
    }
    playArrival(node, arrivalOrigin.current)
    arrivalOrigin.current = null
    // Zero is a parked (display:none) workspace, never the dock's height.
    const report = () => {
      const height = layoutHeight(node)
      if (height > 0) setDockHeight(height)
    }
    report()
    const next = new ResizeObserver(report)
    // react-doctor-disable-next-line react-doctor/effect-needs-cleanup -- a ref callback: the null call above disconnects it
    next.observe(node)
    observer.current = next
  }, [])
  const noteArrivalOrigin = useCallback((rect: DOMRect | null) => {
    arrivalOrigin.current = rect
  }, [])
  return { dockHeight, scrollbarWidth, dockRef, noteArrivalOrigin }
}
