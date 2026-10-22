import { useLayoutEffect, useEffect, useMemo, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { CaretDown, GearIcon } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { NavStack } from './nav-stack'
import {
  SidebarBuildBadgeBand,
  SidebarBuildBadgeLabel,
  useOnNightlySky,
} from './sidebar-build-badge'
import { Button } from '@/components/ui/button'
import { GitPanel } from '@/features/git/components/git-panel'
import { SidebarCarouselFilesPanel } from './sidebar-carousel-files-panel'
import { useCardResizeDrag } from './use-card-resize-drag'
import { useCarouselScrollSync } from './use-carousel-scroll-sync'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { CARD_BOTTOM_INSET_VAR, loadCardHeightFraction } from './sidebar-card-height'
import {
  FALLBACK_SIDEBAR_PANEL_TAB,
  SIDEBAR_PANEL_TABS,
  availableSidebarPanelTabsKey,
} from './sidebar-panel-tabs'

interface SidebarCarouselProps {
  /**
   * Height (px) of the sidebar rail this card floats over (spec §6) —
   * undefined before `ide-shell.tsx`'s own ResizeObserver has measured it
   * once. The card opens at one third of this, then remembers a user drag as
   * a proportion of it (see sidebar-card-height.ts) so it survives a window
   * resize instead of holding a stale pixel value.
   */
  sidebarHeight?: number
  /**
   * Ref to the rail container `SidebarCarousel` and the tree region both sit
   * under in `ide-shell.tsx` (mirrors `pane-sash.tsx`'s own
   * `firstPaneRef`/`secondPaneRef` — refs to SIBLINGS, passed down from the
   * parent that owns them all). During a drag, the live height is written
   * straight onto this node as the `--card-bottom-inset` CSS custom
   * property, which `space-scroller.tsx`'s `SpacePanel` reads via plain CSS
   * inheritance — costing zero React re-renders for every frame of the drag,
   * since nothing but a DOM property write happens until release.
   */
  railRef?: RefObject<HTMLDivElement | null>
  /**
   * Reports the card's own COMMITTED height (px) — mount, a `sidebarHeight`
   * prop resize, and once on drag release. Deliberately NOT called on every
   * frame of a live drag (that traffic goes through `railRef`'s CSS
   * variable instead, exactly so a state update here can't cascade a
   * re-render through the whole tree subtree on every pointermove).
   */
  onHeightChange?: (heightPx: number) => void
}

export function SidebarCarousel({ sidebarHeight, railRef, onHeightChange }: SidebarCarouselProps) {
  const activeTab = useSidebarStore((s) => s.activeTab)
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)
  const containerRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  // The user's own committed open height, as a proportion of `sidebarHeight`
  // — spec §6: "height is kept as a proportion of the rail, so it survives a
  // window resize." Defaults to one third (the spec's own open default) on a
  // first run with nothing persisted yet.
  const [heightFraction, setHeightFraction] = useState(loadCardHeightFraction)
  const cardHeightPx =
    sidebarHeight != null && sidebarHeight > 0
      ? Math.round(sidebarHeight * heightFraction)
      : undefined

  // Fold state (spec §6.4): "the card keeps its head and drops everything
  // under it." In-memory only, deliberately not persisted — unlike
  // `heightFraction` above, which spec §6 explicitly asks to survive a
  // resize, nothing in §6.4 asks a fold to survive a reload. This mirrors
  // space-scroller.tsx's own `SpacePanel` fold precedent exactly (its
  // `folded` state, same reasoning, a different surface).
  const headInk = useOnNightlySky() ? 'text-white' : 'text-muted-foreground'
  const [folded, setFolded] = useState(false)

  // Reported live as "can't parent a chat into a folder": a row/folder near
  // the bottom of a long tree can be scrolled into the space the card's own
  // floating footprint occupies — the tree's ScrollArea box IS correctly
  // shrunk clear of the card (the spacer above), but a card sitting open at
  // its default third-of-the-rail height still claims a lot of vertical
  // room, and a drag's natural approach to a target near the bottom of the
  // list puts the pointer over the card's own surface (Files/Git), not the
  // row underneath — there IS no row underneath; the tree never renders
  // there. `elementsFromPoint` correctly lands on the card and the hit test
  // correctly refuses it — confirmed live, the row nests exactly as it
  // should the moment it is scrolled clear of the card. The fix is not the
  // hit test; it is giving a live row drag the card's own space back, the
  // same "keeps its head, drops everything under it" fold spec §6.4 already
  // defines for the user's own toggle — just driven by `data-row-dragging`
  // (set by use-sidebar-drag.ts for the life of any row drag) instead of a
  // click. Not persisted, same reasoning as `folded` above; restores itself
  // the instant the drag ends.
  const [rowDragging, setRowDragging] = useState(false)
  useEffect(() => {
    const read = () => document.documentElement.hasAttribute('data-row-dragging')
    setRowDragging(read())
    const observer = new MutationObserver(() => setRowDragging(read()))
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-row-dragging'],
    })
    return () => observer.disconnect()
  }, [])
  const cardFolded = folded || rowDragging

  // The RESTING value (mount, sidebarHeight resize, or the recompute a
  // completed drag's committed `heightFraction` triggers) — never fired
  // per-frame from inside a live drag, see the prop doc above.
  useLayoutEffect(() => {
    if (cardHeightPx == null) return
    onHeightChange?.(cardHeightPx)
  }, [cardHeightPx, onHeightChange])

  // Keeps `--card-bottom-inset` in sync with the card's REAL rendered
  // height, pre-paint (`useLayoutEffect`, not `useEffect`) so the tree never
  // flashes a stale inset for one frame. The card floats (`absolute`) and
  // never occupies real flex space in the rail, so this is the ONE number
  // the rows region needs to stay clear of it without overlapping it —
  // measuring the real box rather than trusting `cardHeightPx` (only what
  // the card measures when OPEN — see its own `style` a few lines down)
  // is what makes folding the card actually give that space back to the
  // rows region, instead of reserving a third of the rail's height
  // regardless of what the card is currently showing.
  useLayoutEffect(() => {
    const height = cardRef.current?.getBoundingClientRect().height ?? 0
    railRef?.current?.style.setProperty(CARD_BOTTOM_INSET_VAR, `${height}px`)
  }, [cardHeightPx, cardFolded, railRef])

  // Pointer-drag resize from the top 6px hot zone (spec §6) — see
  // `use-card-resize-drag.ts`'s own doc for the full drag/commit mechanism.
  const handleResizePointerDown = useCardResizeDrag({
    cardRef,
    railRef,
    sidebarHeight,
    cardHeightPx,
    onCommit: setHeightFraction,
  })

  // Tab availability follows the focused workspace context, never the route:
  // a split can focus a branch chat while the route still names project home.
  const availableKey = useFocusedWorkspaceContextStore(availableSidebarPanelTabsKey)
  const visibleHeadTabs = useMemo(() => {
    const available = new Set(availableKey.split(','))
    return SIDEBAR_PANEL_TABS.filter((t) => available.has(t.tab))
  }, [availableKey])
  useEffect(() => {
    // Re-read live: the context is published in a layout effect, which can land
    // after this render's snapshot (e.g. on mount).
    const live = availableSidebarPanelTabsKey(useFocusedWorkspaceContextStore.getState())
    if (!live.split(',').includes(activeTab)) setActiveTab(FALLBACK_SIDEBAR_PANEL_TAB)
  }, [availableKey, activeTab, setActiveTab])

  // Keeps scrollLeft and activeTab in sync in both directions — see
  // `use-carousel-scroll-sync.ts`'s own doc.
  const { armUserGesture, handleScroll } = useCarouselScrollSync(
    containerRef,
    activeTab,
    setActiveTab,
  )

  return (
    // Floats over the tree (spec §6), full width with only a top hairline and
    // no fill, so the translucent sidebar shows through. Bottom is flush: the
    // tree's `--card-bottom-inset` is measured off this card's real height, so
    // any gap would let the last row peek out beneath it.
    <div
      ref={cardRef}
      data-testid="carousel-card"
      // This box's own `absolute` already makes it a positioning ancestor,
      // so the drag-to-trash overlay below (`absolute inset-0`) covers
      // exactly this box without needing a separate `relative`.
      className="absolute inset-x-0 bottom-0 z-10 flex flex-col overflow-hidden border-t"
      style={cardHeightPx != null && !cardFolded ? { height: `${cardHeightPx}px` } : undefined}
    >
      {/* Top 6px hot zone (spec §6) — matches pane-sash.tsx's own
          `h-1.5`/`w-1.5` literally rather than a new value. Lands on the
          card's own top edge (already drawn by its rounded corners/border),
          not a separate visible sash. Hidden while folded (manually or by a
          live row drag): there is no dragged height to adjust when the body
          isn't showing, and the card's own height then collapses to the
          head's (point 4, spec §6.4) rather than reserving the
          last-dragged height. */}
      {!cardFolded && (
        <div
          data-testid="carousel-resize-handle"
          onPointerDown={handleResizePointerDown}
          className="absolute inset-x-0 top-0 z-10 h-1.5 cursor-row-resize touch-none"
        />
      )}
      <NavStack>
        {/* The card is a drawer anchored to the rail's bottom: this head is its footer and the panels above it open upward, so the caret points down while open and up while folded. */}
        {/* `RemovalTray` used to mount here. It must not: this card is
            unmounted whenever the stage is empty or a space is being created
            (ide-shell.tsx's gate), and the tray owns the ONLY commit clock —
            measured live, a held row drained its hairline and no DELETE was
            ever sent. It mounts in the rail itself now. */}
        {/* The body (spec §6.4): folding "drops everything under [the head]" —
            `hidden` (display:none), never a conditional unmount. Both
            Files and Git panels stay mounted the whole time regardless of
            fold (spec §6.2), the same dormancy the pane's own chat/terminal
            surfaces use (agent-chat-pane.tsx) — unmounting here would lose
            each panel's scroll position and re-trigger FileExplorerTree's/
            GitPanel's measured init logic on every unfold. Hidden during a
            live row drag too (`cardFolded`), same as everything else the
            fold touches — the card giving its space back is the whole
            point (see `rowDragging`'s own doc above). */}
        <div
          ref={containerRef}
          onScroll={handleScroll}
          onWheel={armUserGesture}
          onTouchStart={armUserGesture}
          data-sidebar-carousel=""
          className={cn(
            'flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
            cardFolded ? 'hidden' : 'flex',
          )}
        >
          {/* Files panel */}
          <SidebarCarouselFilesPanel />

          {/* Git panel */}
          <div
            data-testid="carousel-panel"
            className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
          >
            <GitPanel />
          </div>
        </div>
        {/* The head (spec §6.1): underline variant, icon only, no labels, no
            divider. justify-start overrides the base tabs list's w-fit
            justify-center; the fold caret sits after the tabs at `ml-auto`,
            on the same head row. */}
        {/* px-1 (4px), not px-2: the row's own height (h-9) already leaves
            exactly 4px of vertical clearance around a size-7 icon-sm control
            (36 - 28 = 8, halved) — px-2 (8px) horizontally doubled that,
            reading as lopsided once this row is the WHOLE visible thing (a
            folded card, not one row among many). Matched, not guessed. */}
        <div
          data-testid="carousel-head"
          className={cn(
            'relative grid h-9 shrink-0 grid-cols-[1fr_auto_1fr] items-center px-1',
            !cardFolded && 'shadow-[inset_0_1px_0_var(--border)]',
          )}
        >
          {/* Paints behind the label and buttons; never affects their layout. */}
          <SidebarBuildBadgeBand className="absolute inset-0 z-0" align="start" />
          {/* Build channel + version, left of the centered buttons. */}
          <div className="relative z-10 flex min-w-0 justify-start pl-2">
            <SidebarBuildBadgeLabel />
          </div>
          {/* Files/Git are plain ghost Buttons, not the Tabs primitive — the
              exact same component/tokens as the fold toggle, Settings, and
              the space marks in the header (icon-sm, rounded-sm, text-muted-foreground,
              hover:bg-sidebar-element-hover). "Selected" reuses the space
              mark's own idiom (opacity-60 when not current) instead of a
              border/shadow/underline of its own. */}
          <div
            data-testid="tabs-list"
            className="relative z-10 flex items-center justify-center gap-1"
          >
            {visibleHeadTabs.map(({ tab, label, Icon }) => {
              const isActive = activeTab === tab
              return (
                <Button
                  key={tab}
                  variant="ghost"
                  size="icon-sm"
                  data-testid={`carousel-head-tab-${tab}`}
                  aria-label={label}
                  aria-pressed={isActive}
                  onClick={() => setActiveTab(tab)}
                  tooltip={label}
                  tooltipSide="bottom"
                  className={cn(
                    'shrink-0 rounded-sm hover:bg-sidebar-element-hover',
                    headInk,
                    !isActive && 'opacity-60',
                  )}
                >
                  <Icon size={16} weight="regular" />
                </Button>
              )
            })}
            <Button
              variant="ghost"
              size="icon-sm"
              data-testid="settings-button"
              aria-label="Settings"
              tooltip="Settings"
              tooltipSide="bottom"
              onClick={() => useUIState.getState().openSettingsDialog()}
              className={cn('shrink-0 rounded-sm hover:bg-sidebar-element-hover', headInk)}
            >
              <GearIcon size={16} />
            </Button>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            data-testid="carousel-fold-toggle"
            // `aria-pressed`/`onClick` stay tied to the REAL toggle only —
            // a drag-fold is not a click and must never be recorded as one.
            aria-pressed={folded}
            aria-label={folded ? 'Expand file explorer' : 'Collapse file explorer'}
            tooltip={folded ? 'Expand file explorer' : 'Collapse file explorer'}
            tooltipSide="bottom"
            onClick={() => setFolded((f) => !f)}
            className={cn(
              'relative z-10 shrink-0 justify-self-end rounded-sm hover:bg-sidebar-element-hover',
              headInk,
            )}
          >
            <CaretDown
              aria-hidden="true"
              data-testid="carousel-fold-caret"
              // The VISUAL follows whichever fold is actually showing — a
              // live row drag reads the same as the user's own toggle here,
              // this control included.
              className={cn('transition-transform', cardFolded && 'rotate-180')}
            />
          </Button>
        </div>
      </NavStack>
    </div>
  )
}
