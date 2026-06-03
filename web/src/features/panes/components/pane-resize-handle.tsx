import { useCallback, useEffect, useRef, useState } from 'react'
import { MIN_PANE_SIZE } from '../constants/pane'

interface PaneResizeHandleProps {
  direction: 'horizontal' | 'vertical'
  index: number
  initialSizes: [number, number]
  splitContainerRef: React.RefObject<HTMLElement | null>
  onResize: (sizes: [number, number]) => void
  onReset?: () => void
}

export function PaneResizeHandle({
  direction,
  index,
  initialSizes,
  splitContainerRef,
  onResize,
  onReset,
}: PaneResizeHandleProps) {
  const [isDragging, setIsDragging] = useState(false)
  const isHorizontal = direction === 'horizontal'
  const startPositionRef = useRef(0)
  const startSizesRef = useRef<[number, number]>(initialSizes)
  const containerSizeRef = useRef(0)
  const latestPositionRef = useRef(0)
  const rafIdRef = useRef<number | null>(null)
  const committedSizesRef = useRef<[number, number]>(initialSizes)
  // Direct DOM refs to the two pane wrappers — set on mousedown, used during drag
  // to bypass CSS var cascade (writing --pane-X-size on the container forces style
  // recalc across all Monaco DOM nodes every frame, killing perf at ~22ms/frame).
  const firstPaneRef = useRef<HTMLElement | null>(null)
  const secondPaneRef = useRef<HTMLElement | null>(null)
  const deductionPxRef = useRef(0)
  // Save the React-managed calc() strings so we can restore them on mouseup.
  // React no-ops on re-render when the style string is unchanged, so we cannot
  // rely on onResize → re-render to fix the inline style we overwrote during drag.
  const originalStylesRef = useRef<[string, string]>(['', ''])

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      const container = splitContainerRef.current
      if (!container) return
      const rect = container.getBoundingClientRect()
      containerSizeRef.current = isHorizontal ? rect.width : rect.height
      startSizesRef.current = initialSizes
      committedSizesRef.current = initialSizes
      startPositionRef.current = isHorizontal ? e.clientX : e.clientY
      latestPositionRef.current = startPositionRef.current

      // Capture pane element refs — each entry is a div.contents wrapper whose
      // first child is the sized pane div (min-h-0 min-w-0 overflow-hidden).
      const dim = isHorizontal ? 'width' : 'height'
      const firstWrapper = container.children[index] as HTMLElement | undefined
      const secondWrapper = container.children[index + 1] as HTMLElement | undefined
      firstPaneRef.current = (firstWrapper?.children[0] as HTMLElement) ?? null
      secondPaneRef.current = (secondWrapper?.children[0] as HTMLElement) ?? null

      // Save the React-managed calc() strings before we override them.
      originalStylesRef.current = [
        firstPaneRef.current?.style.getPropertyValue(dim) ?? '',
        secondPaneRef.current?.style.getPropertyValue(dim) ?? '',
      ]

      // Back-calculate the per-pane handle deduction from actual size so we
      // don't need to know flatEntries.length here.
      const firstPx = isHorizontal
        ? (firstPaneRef.current?.offsetWidth ?? 0)
        : (firstPaneRef.current?.offsetHeight ?? 0)
      deductionPxRef.current = (initialSizes[0] / 100) * containerSizeRef.current - firstPx

      setIsDragging(true)
    },
    [isHorizontal, index, initialSizes, splitContainerRef],
  )

  useEffect(() => {
    if (!isDragging) return

    const dim = isHorizontal ? 'width' : 'height'

    const handleMouseMove = (e: MouseEvent) => {
      latestPositionRef.current = isHorizontal ? e.clientX : e.clientY
      if (rafIdRef.current !== null) return
      rafIdRef.current = requestAnimationFrame(() => {
        rafIdRef.current = null
        const containerSize = containerSizeRef.current
        if (containerSize === 0) return
        const delta = latestPositionRef.current - startPositionRef.current
        const [startFirst, startSecond] = startSizesRef.current
        const total = startFirst + startSecond
        const scaledDelta = (delta / containerSize) * total
        let newFirst = startFirst + scaledDelta
        let newSecond = startSecond - scaledDelta
        if (newFirst < MIN_PANE_SIZE) { newFirst = MIN_PANE_SIZE; newSecond = total - MIN_PANE_SIZE }
        if (newSecond < MIN_PANE_SIZE) { newSecond = MIN_PANE_SIZE; newFirst = total - MIN_PANE_SIZE }
        committedSizesRef.current = [newFirst, newSecond]
        // Write pixel dimensions directly — avoids CSS var cascade through Monaco's
        // ~500 DOM nodes which costs ~17ms/frame at 1080p.
        const deduction = deductionPxRef.current
        firstPaneRef.current?.style.setProperty(dim, `${(newFirst / 100) * containerSize - deduction}px`)
        secondPaneRef.current?.style.setProperty(dim, `${(newSecond / 100) * containerSize - deduction}px`)
      })
    }

    const handleMouseUp = () => {
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current)
        rafIdRef.current = null
      }
      const [s0, s1] = committedSizesRef.current
      const [orig0, orig1] = originalStylesRef.current
      // 1. Update CSS vars to committed values so the calc() expressions resolve correctly.
      const container = splitContainerRef.current
      if (container) {
        container.style.setProperty(`--pane-${index}-size`, String(s0))
        container.style.setProperty(`--pane-${index + 1}-size`, String(s1))
      }
      // 2. Restore the original calc() strings (not removeProperty).
      //    removeProperty leaves the element with no width, and React won't re-set it
      //    because the calc() string is identical across renders (no-op reconciler).
      //    Restoring the string means the browser re-evaluates calc() against the
      //    updated CSS var — correct size, no flash, no collapse.
      if (orig0 && firstPaneRef.current) firstPaneRef.current.style.setProperty(dim, orig0)
      if (orig1 && secondPaneRef.current) secondPaneRef.current.style.setProperty(dim, orig1)
      firstPaneRef.current = null
      secondPaneRef.current = null
      onResize(committedSizesRef.current)
      setIsDragging(false)
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current)
        rafIdRef.current = null
      }
    }
  }, [isDragging, isHorizontal, index, onResize, splitContainerRef])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const relevant = isHorizontal
        ? ['ArrowLeft', 'ArrowRight']
        : ['ArrowUp', 'ArrowDown']
      if (!relevant.includes(e.key)) return
      e.preventDefault()
      const [first, second] = initialSizes
      const total = first + second
      const step = (e.shiftKey ? 0.10 : 0.02) * total
      const delta = (e.key === 'ArrowRight' || e.key === 'ArrowDown') ? step : -step
      const newFirst = Math.max(MIN_PANE_SIZE, Math.min(total - MIN_PANE_SIZE, first + delta))
      onResize([newFirst, total - newFirst])
    },
    [isHorizontal, initialSizes, onResize],
  )

  return (
    <div
      className={`group relative flex shrink-0 items-center justify-center ${
        isHorizontal ? 'h-full w-2 cursor-col-resize' : 'h-2 w-full cursor-row-resize'
      }`}
      onMouseDown={handleMouseDown}
      onDoubleClick={onReset}
      onKeyDown={handleKeyDown}
      role="separator"
      aria-orientation={isHorizontal ? 'vertical' : 'horizontal'}
      aria-valuenow={Math.round(initialSizes[0])}
      aria-valuemin={MIN_PANE_SIZE}
      aria-valuemax={100 - MIN_PANE_SIZE}
      tabIndex={0}
    >
      <div
        className={`${isDragging ? 'bg-accent' : 'bg-transparent group-hover:bg-accent'} ${
          isHorizontal ? 'h-full w-px' : 'h-px w-full'
        }`}
      />
      {isDragging && (
        <div
          className={`fixed inset-0 z-50 ${isHorizontal ? 'cursor-col-resize' : 'cursor-row-resize'}`}
        />
      )}
    </div>
  )
}
