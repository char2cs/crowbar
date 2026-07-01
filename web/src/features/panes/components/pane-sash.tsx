import { useCallback, useEffect, useRef, type PointerEvent as ReactPointerEvent } from 'react'
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

  // Track whether a drag is currently active (for unmount cleanup).
  const isDraggingRef = useRef<boolean>(false)

  // Stable refs for listener functions so the same instance is always used for
  // both addEventListener and removeEventListener (including unmount cleanup).
  const onPointerMoveRef = useRef<((e: globalThis.PointerEvent) => void) | null>(null)
  const onPointerUpRef = useRef<((e: globalThis.PointerEvent) => void) | null>(null)

  // Capture the sizes and callbacks in refs so the stable listener callbacks
  // always see the latest values without needing to be recreated.
  const sizesRef = useRef(sizes)
  const onResizeCommitRef = useRef(onResizeCommit)
  sizesRef.current = sizes
  onResizeCommitRef.current = onResizeCommit

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

  /**
   * Shared teardown: removes window listeners and clears the resizing
   * attribute. Safe to call multiple times (idempotent via isDraggingRef).
   */
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

    document.documentElement.removeAttribute('data-pane-resizing')
    window.dispatchEvent(new CustomEvent('pane-resize-end'))
  }, [])

  /**
   * Unmount-mid-drag cleanup: if a drag is in progress when the component is
   * removed from the tree, tear down the window listeners and clear the
   * resizing attribute so the rest of the app is not left in a broken state.
   * This is a NO-OP when no drag is active.
   */
  useEffect(() => {
    return () => {
      // Only the teardown path matters here; teardownDrag is idempotent.
      if (isDraggingRef.current) {
        teardownDrag()
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

      // Build stable listener instances and store in refs before attaching.
      const onMove = (ev: globalThis.PointerEvent) => {
        const c = containerRef.current
        if (!c) return
        const rect = c.getBoundingClientRect()
        const cPx = measureContainerPx()
        containerPxRef.current = cPx
        const pointerPx = isHorizontal ? ev.clientX - rect.left : ev.clientY - rect.top
        const [fPct, sPct] = pxToPercents(cPx, pointerPx, MIN_PANE_SIZE)
        const [fPx, sPx] = percentsToPx(cPx, [fPct, sPct])
        liveFirstPx.current = fPx
        const first = firstPaneRef.current
        const second = secondPaneRef.current
        if (first) first.style.flexBasis = `${fPx}px`
        if (second) second.style.flexBasis = `${sPx}px`
      }

      const onUp = (ev: globalThis.PointerEvent) => {
        const cPx = containerPxRef.current
        const finalSizes =
          cPx > 0 ? pxToPercents(cPx, liveFirstPx.current, MIN_PANE_SIZE) : sizesRef.current

        // Drop the inline imperative sizes so React's percentage flex-basis
        // (driven by the committed store state) takes over again.
        if (firstPaneRef.current) firstPaneRef.current.style.flexBasis = ''
        if (secondPaneRef.current) secondPaneRef.current.style.flexBasis = ''

        teardownDrag()
        onResizeCommitRef.current(finalSizes)
        ev.preventDefault()
      }

      onPointerMoveRef.current = onMove
      onPointerUpRef.current = onUp

      isDraggingRef.current = true
      document.documentElement.setAttribute('data-pane-resizing', '1')
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onUp)
      window.addEventListener('pointercancel', onUp)
    },
    [containerRef, firstPaneRef, secondPaneRef, isHorizontal, measureContainerPx, teardownDrag],
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
      )}
    />
  )
}
