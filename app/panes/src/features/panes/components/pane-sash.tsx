import { useCallback, useEffect, useRef, type PointerEvent as ReactPointerEvent } from 'react'
import { cn } from '@/lib/utils'
import { MIN_PANE_SIZE } from '../constants/pane'
import { percentsToPx, pxToPercents } from '../lib/split-sizing'

interface PaneSashProps {
  direction: 'horizontal' | 'vertical'
  sizes: [number, number]
  containerRef: React.RefObject<HTMLDivElement | null>
  firstPaneRef: React.RefObject<HTMLDivElement | null>
  secondPaneRef: React.RefObject<HTMLDivElement | null>
  onResizeCommit: (sizes: [number, number]) => void
  minPx?: number
  className?: string
}

export function PaneSash({
  direction,
  sizes,
  containerRef,
  firstPaneRef,
  secondPaneRef,
  onResizeCommit,
  minPx = MIN_PANE_SIZE,
  className,
}: PaneSashProps) {
  const isHorizontal = direction === 'horizontal'
  const sashRef = useRef<HTMLDivElement>(null)
  const liveFirstPx = useRef<number>(0)
  const containerPxRef = useRef<number>(0)

  const isDraggingRef = useRef<boolean>(false)
  const hasMovedRef = useRef<boolean>(false)
  const renderedBasisRef = useRef<[string, string]>(['', ''])

  const onPointerMoveRef = useRef<((e: globalThis.PointerEvent) => void) | null>(null)
  const onPointerUpRef = useRef<((e: globalThis.PointerEvent) => void) | null>(null)

  const sizesRef = useRef(sizes)
  const onResizeCommitRef = useRef(onResizeCommit)
  sizesRef.current = sizes
  onResizeCommitRef.current = onResizeCommit

  const measureContainerPx = useCallback((): number => {
    const container = containerRef.current
    if (!container) return 0
    const rect = container.getBoundingClientRect()
    const total = isHorizontal ? rect.width : rect.height
    const sash = sashRef.current
    const sashPx = sash ? (isHorizontal ? sash.offsetWidth : sash.offsetHeight) : 0
    return Math.max(0, total - sashPx)
  }, [containerRef, isHorizontal])

  const teardownDrag = useCallback(() => {
    if (!isDraggingRef.current) return
    isDraggingRef.current = false

    const onMove = onPointerMoveRef.current
    const onUp = onPointerUpRef.current
    if (onMove) window.removeEventListener('pointermove', onMove)
    if (onUp) {
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
    }

    if (hasMovedRef.current) {
      document.documentElement.removeAttribute('data-pane-resizing')
      window.dispatchEvent(new CustomEvent('pane-resize-end'))
    }
    if (hasMovedRef.current) {
      const [firstBasis, secondBasis] = renderedBasisRef.current
      if (firstPaneRef.current) firstPaneRef.current.style.flexBasis = firstBasis
      if (secondPaneRef.current) secondPaneRef.current.style.flexBasis = secondBasis
    }
    hasMovedRef.current = false
  }, [firstPaneRef, secondPaneRef])

  useEffect(() => {
    return () => {
      if (isDraggingRef.current) {
        teardownDrag()
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- unmount-only: teardownDrag reads refs, so the first one is as good as the latest.
  }, [])

  const handlePointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return
      e.preventDefault()
      const container = containerRef.current
      if (!container) return
      const containerPx = measureContainerPx()
      containerPxRef.current = containerPx
      const [firstPx] = percentsToPx(containerPx, sizesRef.current)
      liveFirstPx.current = firstPx
      hasMovedRef.current = false
      renderedBasisRef.current = [
        firstPaneRef.current?.style.flexBasis ?? '',
        secondPaneRef.current?.style.flexBasis ?? '',
      ]

      const onMove = (ev: globalThis.PointerEvent) => {
        const c = containerRef.current
        if (!c) return
        if (!hasMovedRef.current) {
          hasMovedRef.current = true
          document.documentElement.setAttribute('data-pane-resizing', '1')
        }
        const rect = c.getBoundingClientRect()
        const cPx = measureContainerPx()
        containerPxRef.current = cPx
        const pointerPx = isHorizontal ? ev.clientX - rect.left : ev.clientY - rect.top
        const [fPct, sPct] = pxToPercents(cPx, pointerPx, minPx)
        const [fPx, sPx] = percentsToPx(cPx, [fPct, sPct])
        liveFirstPx.current = fPx
        const first = firstPaneRef.current
        const second = secondPaneRef.current
        if (first) first.style.flexBasis = `${fPx}px`
        if (second) second.style.flexBasis = `${sPx}px`
      }

      const onUp = (ev: globalThis.PointerEvent) => {
        if (!hasMovedRef.current) {
          teardownDrag()
          ev.preventDefault()
          return
        }

        const cPx = containerPxRef.current
        const finalSizes =
          cPx > 0 ? pxToPercents(cPx, liveFirstPx.current, minPx) : sizesRef.current

        teardownDrag()
        onResizeCommitRef.current(finalSizes)
        ev.preventDefault()
      }

      onPointerMoveRef.current = onMove
      onPointerUpRef.current = onUp

      isDraggingRef.current = true
      // react-doctor-disable-next-line effect-needs-cleanup -- this is an event handler, not an effect; listeners are removed by `teardownDrag`, which the unmount effect also runs mid-drag.
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onUp)
      window.addEventListener('pointercancel', onUp)
    },
    [
      containerRef,
      firstPaneRef,
      secondPaneRef,
      isHorizontal,
      measureContainerPx,
      minPx,
      teardownDrag,
    ],
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
        isHorizontal ? 'w-1.5 cursor-col-resize' : 'h-1.5 w-full cursor-row-resize',
        className,
      )}
    />
  )
}
