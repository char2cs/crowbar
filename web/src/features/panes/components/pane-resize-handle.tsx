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
      setIsDragging(true)
    },
    [isHorizontal, initialSizes, splitContainerRef],
  )

  useEffect(() => {
    if (!isDragging) return

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
        const container = splitContainerRef.current
        if (container) {
          container.style.setProperty(`--pane-${index}-size`, String(newFirst))
          container.style.setProperty(`--pane-${index + 1}-size`, String(newSecond))
        }
      })
    }

    const handleMouseUp = () => {
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current)
        rafIdRef.current = null
      }
      const container = splitContainerRef.current
      if (container) {
        container.style.removeProperty(`--pane-${index}-size`)
        container.style.removeProperty(`--pane-${index + 1}-size`)
      }
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
