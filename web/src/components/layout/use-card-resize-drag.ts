import { useCallback, useEffect, useRef } from 'react'
import type { PointerEvent as ReactPointerEvent, RefObject } from 'react'
import {
  CARD_BOTTOM_INSET_VAR,
  DEFAULT_CARD_HEIGHT_FRACTION,
  clampCardHeightFraction,
  saveCardHeightFraction,
} from './sidebar-card-height'

interface UseCardResizeDragOptions {
  cardRef: RefObject<HTMLDivElement | null>
  railRef?: RefObject<HTMLDivElement | null>
  sidebarHeight?: number
  cardHeightPx: number | undefined
  /** Called once, on release, with the newly committed height fraction. */
  onCommit: (fraction: number) => void
}

/**
 * Pointer-drag resize from the card's top 6px hot zone (spec §6) — split out
 * of `SidebarCarousel` because it is a self-contained drag/CSS-write loop
 * that only ever touches its own refs and the two values passed in, never
 * the carousel's tab/fold/scroll state. Mirrors pane-sash.tsx's/
 * sidebar-split-pane.tsx's own pattern — track window pointermove/up from
 * pointerdown, coalesce the live size to one DOM write per animation frame,
 * commit once on release — rather than importing pane-sash.tsx itself: that
 * component drags a flex-basis between two sibling panes, this drags one
 * floating element's own height against a rail it does not share layout
 * with.
 *
 * Every live-frame write is imperative DOM/CSS only — the card's own
 * `cardRef.current.style.height` AND `railRef`'s `--card-bottom-inset`
 * custom property, which `SpacePanel` reads via CSS inheritance
 * (space-scroller.tsx). `onCommit` (React state, one level up in
 * ide-shell.tsx via SidebarCarousel's own `onHeightChange`) fires exactly
 * once, on release, deliberately: calling it per frame previously
 * re-rendered ide-shell.tsx and, since none of SidebarTreeSurface/
 * SpaceScroller/SpacePanel/SidebarTree/SidebarRow are memoized, every
 * visible row in the active project on every frame of a drag.
 */
export function useCardResizeDrag({
  cardRef,
  railRef,
  sidebarHeight,
  cardHeightPx,
  onCommit,
}: UseCardResizeDragOptions): (e: ReactPointerEvent<HTMLDivElement>) => void {
  const activeDragCleanupRef = useRef<(() => void) | null>(null)

  const handleResizePointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return
      const rail = sidebarHeight
      if (!rail || rail <= 0) return
      e.preventDefault()
      const startY = e.clientY
      const startHeight = cardHeightPx ?? Math.round(rail * DEFAULT_CARD_HEIGHT_FRACTION)
      let liveHeight = startHeight
      let moved = false
      let animationFrame = 0

      const applyLiveHeight = () => {
        animationFrame = 0
        if (cardRef.current) cardRef.current.style.height = `${liveHeight}px`
        railRef?.current?.style.setProperty(CARD_BOTTOM_INSET_VAR, `${liveHeight}px`)
      }

      const teardown = () => {
        window.removeEventListener('pointermove', onMove)
        window.removeEventListener('pointerup', onUp)
        window.removeEventListener('pointercancel', onUp)
        if (animationFrame !== 0) cancelAnimationFrame(animationFrame)
        activeDragCleanupRef.current = null
        if (moved) {
          document.documentElement.removeAttribute('data-pane-resizing')
          window.dispatchEvent(new CustomEvent('pane-resize-end'))
        }
      }

      const onMove = (ev: globalThis.PointerEvent) => {
        if (!moved) {
          moved = true
          document.documentElement.setAttribute('data-pane-resizing', '1')
        }
        // The card is anchored to the RAIL's bottom edge, so dragging the top
        // edge up (negative delta) grows it.
        const delta = ev.clientY - startY
        const raw = startHeight - delta
        const clampedFraction = clampCardHeightFraction(raw / rail)
        liveHeight = Math.round(rail * clampedFraction)
        if (animationFrame === 0) animationFrame = requestAnimationFrame(applyLiveHeight)
      }

      const onUp = () => {
        const wasMoved = moved
        if (animationFrame !== 0) {
          cancelAnimationFrame(animationFrame)
          applyLiveHeight()
        }
        teardown()
        if (!wasMoved) return
        const fraction = clampCardHeightFraction(liveHeight / rail)
        onCommit(fraction)
        saveCardHeightFraction(fraction)
      }

      activeDragCleanupRef.current = teardown
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onUp)
      window.addEventListener('pointercancel', onUp)
    },
    [sidebarHeight, cardHeightPx, railRef, cardRef, onCommit],
  )

  // Unmount-mid-drag safety (mirrors pane-sash.tsx): stray window listeners
  // and the global resizing attribute must not survive this component going
  // away — e.g. the sidebar auto-collapsing, or a route change, mid-drag.
  useEffect(() => {
    return () => activeDragCleanupRef.current?.()
  }, [])

  return handleResizePointerDown
}
