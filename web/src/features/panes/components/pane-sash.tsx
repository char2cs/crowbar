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
 * Both are entered lazily on the first real `pointermove`, not on
 * `pointerdown`: a plain click (down+up, no movement) must be a total no-op,
 * since the attribute globally GPU-layer-promotes every mounted
 * `.monaco-editor` (including off-screen/retained ones), and toggling that
 * on a click that resizes nothing can visibly glitch WKWebView's compositor.
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
  // Whether the pointer has actually moved since pointerdown. A plain click
  // (pointerdown -> pointerup with zero pointermove) must be a no-op: it must
  // not toggle `data-pane-resizing`, since that attribute globally promotes
  // every mounted `.monaco-editor` (including off-screen/retained ones) onto
  // a GPU compositing layer and back within one tick, which can visibly glitch
  // WKWebView's compositor even though nothing was actually resized.
  const hasMovedRef = useRef<boolean>(false)

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

    // Only touch the shared attribute/event if this drag ever actually
    // entered the resizing state (i.e. the pointer moved at least once).
    if (hasMovedRef.current) {
      document.documentElement.removeAttribute('data-pane-resizing')
      window.dispatchEvent(new CustomEvent('pane-resize-end'))
    }
    hasMovedRef.current = false
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
      hasMovedRef.current = false

      // Build stable listener instances and store in refs before attaching.
      const onMove = (ev: globalThis.PointerEvent) => {
        const c = containerRef.current
        if (!c) return
        // Enter the resizing state lazily, on the first real move, rather
        // than on pointerdown. A bare click never reaches this line.
        if (!hasMovedRef.current) {
          hasMovedRef.current = true
          document.documentElement.setAttribute('data-pane-resizing', '1')
        }
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
        // A plain click (no pointermove ever fired) must be a total no-op:
        // no attribute toggle, no pane-resize-end, no size commit.
        if (!hasMovedRef.current) {
          teardownDrag()
          ev.preventDefault()
          return
        }

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
      // react-doctor-disable-next-line effect-needs-cleanup -- this is an event handler, not an effect; listeners are removed via `teardownDrag` + the unmount effect (l.112-116).
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
