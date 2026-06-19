import { useCallback, useRef, type PointerEvent as ReactPointerEvent } from 'react'
import { cn } from '@/lib/utils'
import { MIN_PANE_SIZE } from '../constants/pane'
import { percentsToPx, pxToPercents } from '../lib/split-sizing'

interface PaneSashProps {
  direction: 'horizontal' | 'vertical'
  /** Current authoritative split ratio `[firstPct, secondPct]`. */
  sizes: [number, number]
  /** Element whose size defines 100% (the split container). */
  containerRef: React.RefObject<HTMLDivElement | null>
  /** Sibling pane elements to mutate imperatively during the drag. */
  firstPaneRef: React.RefObject<HTMLDivElement | null>
  secondPaneRef: React.RefObject<HTMLDivElement | null>
  /** Commit final percentages on pointer-up. */
  onResizeCommit: (sizes: [number, number]) => void
}

/**
 * VSCode-style imperative pixel sash. During a drag the two sibling pane
 * elements have their flex-basis set directly in pixels (no React state, no
 * store writes) for reflow-free movement. On pointer-up the final ratio is
 * committed back to the authoritative layout tree as percentages.
 *
 * The `data-pane-resizing` attribute on <html> and the `pane-resize-end`
 * window event are preserved exactly — the Monaco perf fix depends on them.
 */
export function PaneSash({
  direction,
  sizes,
  containerRef,
  firstPaneRef,
  secondPaneRef,
  onResizeCommit,
}: PaneSashProps) {
  const isHorizontal = direction === 'horizontal'
  const sashRef = useRef<HTMLDivElement>(null)
  // The latest pixel size of the first pane during a drag, committed on pointer-up.
  const liveFirstPx = useRef<number>(0)
  const containerPxRef = useRef<number>(0)

  // Available space for the two panes = container minus the sash's own size,
  // so the panes plus the sash fit exactly without overflow.
  const measureContainerPx = useCallback((): number => {
    const container = containerRef.current
    if (!container) return 0
    const rect = container.getBoundingClientRect()
    const total = isHorizontal ? rect.width : rect.height
    const sash = sashRef.current
    const sashPx = sash ? (isHorizontal ? sash.offsetWidth : sash.offsetHeight) : 0
    return Math.max(0, total - sashPx)
  }, [containerRef, isHorizontal])

  const endDrag = useCallback(() => {
    document.documentElement.removeAttribute('data-pane-resizing')
    window.dispatchEvent(new CustomEvent('pane-resize-end'))
  }, [])

  const handlePointerMove = useCallback(
    (e: globalThis.PointerEvent) => {
      const container = containerRef.current
      if (!container) return
      const rect = container.getBoundingClientRect()
      const containerPx = measureContainerPx()
      containerPxRef.current = containerPx
      const pointerPx = isHorizontal ? e.clientX - rect.left : e.clientY - rect.top
      const [firstPct, secondPct] = pxToPercents(containerPx, pointerPx, MIN_PANE_SIZE)
      const [firstPx, secondPx] = percentsToPx(containerPx, [firstPct, secondPct])
      liveFirstPx.current = firstPx
      const first = firstPaneRef.current
      const second = secondPaneRef.current
      if (first) first.style.flexBasis = `${firstPx}px`
      if (second) second.style.flexBasis = `${secondPx}px`
    },
    [containerRef, firstPaneRef, secondPaneRef, isHorizontal, measureContainerPx],
  )

  const handlePointerUp = useCallback(
    (e: globalThis.PointerEvent) => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
      window.removeEventListener('pointercancel', handlePointerUp)

      const containerPx = containerPxRef.current
      const finalSizes =
        containerPx > 0 ? pxToPercents(containerPx, liveFirstPx.current, MIN_PANE_SIZE) : sizes

      // Drop the inline imperative sizes so React's percentage flex-basis
      // (driven by the committed store state) takes over again.
      if (firstPaneRef.current) firstPaneRef.current.style.flexBasis = ''
      if (secondPaneRef.current) secondPaneRef.current.style.flexBasis = ''

      endDrag()
      onResizeCommit(finalSizes)
      e.preventDefault()
    },
    [handlePointerMove, endDrag, onResizeCommit, sizes, firstPaneRef, secondPaneRef],
  )

  const handlePointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return
      e.preventDefault()
      const container = containerRef.current
      if (!container) return
      const containerPx = measureContainerPx()
      containerPxRef.current = containerPx
      const [firstPx] = percentsToPx(containerPx, sizes)
      liveFirstPx.current = firstPx

      document.documentElement.setAttribute('data-pane-resizing', '1')
      window.addEventListener('pointermove', handlePointerMove)
      window.addEventListener('pointerup', handlePointerUp)
      window.addEventListener('pointercancel', handlePointerUp)
    },
    [containerRef, sizes, handlePointerMove, handlePointerUp, measureContainerPx],
  )

  return (
    <div
      ref={sashRef}
      role="separator"
      aria-orientation={isHorizontal ? 'vertical' : 'horizontal'}
      data-slot="resizable-handle"
      onPointerDown={handlePointerDown}
      className={cn(
        'relative z-10 flex shrink-0 items-center justify-center ring-offset-background',
        'transition-colors hover:bg-border/60',
        isHorizontal
          ? 'w-1.5 cursor-col-resize'
          : 'h-1.5 w-full cursor-row-resize',
      )}
    />
  )
}
