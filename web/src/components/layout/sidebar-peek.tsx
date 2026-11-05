import {
  type CSSProperties,
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { useSidebarPeekStore } from '@/features/window/stores/sidebar-peek-store'
import { cn } from '@/utils/cn'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

/** The properties `transition-transform` animates that actually move the card:
 *  the slide is written as `translate-x-*`, i.e. the CSS `translate` property. */
const SLIDE_PROPERTIES = new Set(['translate', 'transform'])
/** Gap between the peeked card and the window edges (Tailwind `inset-2`). */
const PEEK_MARGIN_PX = 8
/** Edge distance that summons the peek. A hit TEST, not an element: an element would swallow the
 *  editor's wheel, clicks and selection there (and Monaco's scrollbar when docked right). */
const EDGE_TRIGGER_PX = 6
/** Top chrome band — never summon from behind the traffic lights / drag region. */
const CHROME_BAND_PX = CHROME_ROW_HEIGHT_PX
/** Clears the bottom corner-resize zone, which is wider than the edge band. */
const BOTTOM_SAFE_PX = 16

interface SidebarPeekProps {
  /** The sidebar panel is collapsed. Only then is peeking possible. */
  hidden: boolean
  side: 'left' | 'right'
  /** The user's remembered sidebar width — the peeked card matches it. */
  width: number
  children: ReactNode
}

// While the sidebar is collapsed, the pointer at its window edge slides a floating copy over the
// editor without changing panel geometry. The wrappers never move in the tree (only classes change)
// so the sidebar never remounts; the card wears the popover surface since vibrancy can't float.
export function SidebarPeek({ hidden, side, width, children }: SidebarPeekProps) {
  const [hovered, setHovered] = useState(false)
  const setOccupied = useSidebarPeekStore((s) => s.setOccupied)
  const cardRef = useRef<HTMLDivElement>(null)
  const peeking = hidden && hovered
  const isRight = side === 'right'

  // Hit-tested, so a closed peek costs the editor nothing. Once open, the card's footprint counts as
  // still peeking, so no close timer is needed: there is no gap to cross.
  useEffect(() => {
    if (!hidden) {
      // Pinning the sidebar open must not leave a stale hover behind, or the
      // card reappears unbidden — and undismissable — next time it is hidden.
      setHovered(false)
      return
    }

    const onPointerMove = (event: PointerEvent) => {
      const { clientX: x, clientY: y } = event
      const inTriggerBand =
        (isRight ? x >= window.innerWidth - EDGE_TRIGGER_PX : x <= EDGE_TRIGGER_PX) &&
        y >= CHROME_BAND_PX &&
        y <= window.innerHeight - BOTTOM_SAFE_PX
      // Computed rather than measured: reading the card's rect on every pointer
      // move would force layout. Only meaningful once open, since the card's
      // footprint is a large slice of the editor.
      const cardLeft = isRight ? window.innerWidth - PEEK_MARGIN_PX - width : PEEK_MARGIN_PX
      const overCard =
        peeking && x >= cardLeft - PEEK_MARGIN_PX && x <= cardLeft + width + PEEK_MARGIN_PX
      setHovered(inTriggerBand || overCard)
    }
    // The pointer can leave without a final move — out of the window, or to
    // another app — which would otherwise strand the card on screen.
    const close = () => setHovered(false)

    document.addEventListener('pointermove', onPointerMove, { passive: true })
    document.addEventListener('pointerleave', close)
    window.addEventListener('blur', close)
    return () => {
      document.removeEventListener('pointermove', onPointerMove)
      document.removeEventListener('pointerleave', close)
      window.removeEventListener('blur', close)
    }
  }, [hidden, peeking, isRight, width])

  // Native webviews step aside while the card is on screen, including its
  // slide-out, which ends at transitionend. With no transition (reduced
  // motion, docked) that event never comes, so the card is gone at once.
  useLayoutEffect(() => {
    if (peeking) {
      setOccupied(true)
      return
    }
    const card = cardRef.current
    if (
      !card ||
      !getComputedStyle(card)
        .transitionDuration.split(',')
        .some((d) => parseFloat(d) > 0)
    ) {
      setOccupied(false)
      return
    }
    const done = (event: Event) => {
      if (event.target === card && SLIDE_PROPERTIES.has((event as TransitionEvent).propertyName)) {
        setOccupied(false)
      }
    }
    card.addEventListener('transitionend', done)
    return () => card.removeEventListener('transitionend', done)
  }, [peeking, setOccupied])
  useEffect(() => () => setOccupied(false), [setOccupied])

  return (
    <div
      data-sidebar-peek=""
      data-state={hidden ? (peeking ? 'peeking' : 'closed') : 'docked'}
      className={cn('flex min-h-0 flex-col', hidden ? 'contents' : 'h-full')}
    >
      <div
        ref={cardRef}
        // Parked off-screen it is still in the DOM, so without this a Tab from
        // the editor walks into an invisible sidebar — and anything typed after
        // the card slides away keeps landing in a search field nobody can see.
        inert={hidden && !peeking}
        className={cn(
          'flex min-h-0 flex-col',
          hidden
            ? cn(
                // z-40 clears the z-[1] content pane and in-pane search
                // affordances while staying below the z-50 portal layer.
                'fixed inset-y-2 z-40 w-(--peek-width) max-w-[calc(100vw-1rem)]',
                isRight ? 'right-2' : 'left-2',
                // The floating-surface recipe (popover.tsx): bg-clip-padding keeps the background out
                // of the translucent border; the inset ::before carries the hairline highlight.
                'overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg/5 outline-none not-dark:bg-clip-padding',
                // The command palette's `before:bg-muted` wash takes the glare off light-mode white and
                // lifts the card in dark (--muted is 4% black/white), with no per-theme override.
                'before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-xl)-1px)] before:bg-muted before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)]',
                // Transform only, so it composites. Reduced motion is handled
                // globally by the kill-switch in index.css.
                'transition-transform duration-200 ease-[cubic-bezier(0.4,0,0.2,1)]',
                peeking
                  ? // will-change only while it is on screen — holding a
                    // full-height compositor layer for a parked, invisible card
                    // is pure cost.
                    'translate-x-0 will-change-transform'
                  : // The extra 1rem clears the margin and the shadow, so no
                    // sliver of the card shows while it is parked off-screen.
                    isRight
                    ? 'translate-x-[calc(100%+1rem)]'
                    : 'translate-x-[calc(-100%-1rem)]',
              )
            : 'h-full',
        )}
        style={hidden ? ({ '--peek-width': `${width}px` } as CSSProperties) : undefined}
      >
        {children}
      </div>
    </div>
  )
}
