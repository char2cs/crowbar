import { type CSSProperties, type ReactNode, useEffect, useState } from 'react'
import { cn } from '@/utils/cn'
import { IS_MAC } from '@/utils/platform'

/** Gap between the peeked card and the window edges (Tailwind `inset-2`). */
const PEEK_MARGIN_PX = 8
/**
 * How close to the window edge the pointer must come to summon the peek.
 *
 * The window is natively framed and resizable (`decorations` defaults to true,
 * `resizable: true` in tauri.conf.json), so AppKit owns a resize tracking band a
 * few points wide at the frame. Those points are lost to us either way, which is
 * why this is a hit TEST rather than a hit TARGET: an actual element here would
 * sit on top of the editor and swallow its wheel, clicks and text selection in
 * that column — and with the sidebar docked right it would cover most of
 * Monaco's 14px vertical scrollbar, making it undraggable.
 */
const EDGE_TRIGGER_PX = 6
/** Top chrome band — never summon from behind the traffic lights / drag region. */
const CHROME_BAND_PX = IS_MAC ? 44 : 34
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

/**
 * Hover-to-peek host for the sidebar.
 *
 * While the sidebar is hidden, bringing the pointer to the window edge it is
 * docked against slides a floating copy of it in over the editor; moving away
 * slides it out. Peeking is not pinning: it changes no panel geometry, so the
 * editor never reflows, the panel stays collapsed and `sidebarOpen` stays false.
 *
 * The wrappers render in EVERY state and never move in the tree; only their
 * classes change. That is load-bearing — rendering `children` into a separate
 * overlay container when hidden would unmount and rebuild the entire sidebar
 * (workspace tree, carousel scroll offset, file explorer, agent chat list),
 * which is the same defect that made flipping the sidebar's side so expensive.
 * See the comment above `sidebarPanel` in ide-shell.tsx.
 *
 * The docked sidebar is deliberately transparent so the native window vibrancy
 * shows through; a floating card cannot be, or the editor would read through it,
 * so the peek wears the app's standard popover surface instead.
 */
export function SidebarPeek({ hidden, side, width, children }: SidebarPeekProps) {
  const [hovered, setHovered] = useState(false)
  const peeking = hidden && hovered
  const isRight = side === 'right'

  // Hit-tested against the pointer rather than served by an element, so the peek
  // costs the editor nothing while it is closed. Once open, the card's own
  // footprint counts as "still peeking" — which is what removes the need for a
  // close timer: there is no gap between the trigger band and the card to cross.
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

  return (
    <div
      data-sidebar-peek=""
      data-state={hidden ? (peeking ? 'peeking' : 'closed') : 'docked'}
      className={cn('flex min-h-0 flex-col', hidden ? 'contents' : 'h-full')}
    >
      <div
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
                // The app's floating-surface recipe (popover.tsx / dialog.tsx):
                // bg-clip-padding stops the background bleeding through the
                // translucent light-mode border, and the inset ::before carries
                // the hairline highlight at the parent radius minus its 1px.
                'overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-lg/5 outline-none not-dark:bg-clip-padding',
                // The `before:bg-muted` wash is the command palette's recipe
                // (command.tsx) for a large surface that would otherwise glare:
                // --popover is pure white in light mode, which reads far
                // brighter than the docked sidebar's vibrancy glass it stands in
                // for. --muted is 4% black in light and 4% WHITE in dark, so one
                // token both takes the glare off and lifts the card off the
                // background — no per-theme override.
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
