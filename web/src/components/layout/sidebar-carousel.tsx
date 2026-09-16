import { useLayoutEffect, useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { useMatch } from '@tanstack/react-router'
import { CaretDown, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { NavStack } from './nav-stack'
import { Button } from '@/components/ui/button'
import { GitPanel } from '@/features/git/components/git-panel'
import { RemovalTray } from './removal-tray'
import { SidebarCarouselFilesPanel } from './sidebar-carousel-files-panel'
import { useCardResizeDrag } from './use-card-resize-drag'
import { useCarouselScrollSync } from './use-carousel-scroll-sync'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { CARD_BOTTOM_INSET_VAR, loadCardHeightFraction } from './sidebar-card-height'

// The head's two glyphs (spec §6.1), in the carousel's own panel order
// (Files, then Git — see use-carousel-scroll-sync.ts's own TABS).
const HEAD_TABS: {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
}[] = [
  { tab: 'files', label: 'Files', Icon: FolderOpen },
  { tab: 'git', label: 'Git', Icon: GitBranch },
]

interface SidebarCarouselProps {
  activeWorkspaceRepoPath: string
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

export function SidebarCarousel({
  activeWorkspaceRepoPath,
  sidebarHeight,
  railRef,
  onHeightChange,
}: SidebarCarouselProps) {
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

  // Git has no meaning without a repo, and the project-home route has no
  // active workspace — carried over verbatim from the old SidebarTabBar,
  // which is retired now that the head lives here (spec §6.1).
  const isHomeRoute = Boolean(useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false }))
  useEffect(() => {
    if (isHomeRoute && activeTab === 'git') {
      setActiveTab('files')
    }
  }, [isHomeRoute, activeTab, setActiveTab])
  const visibleHeadTabs = isHomeRoute ? HEAD_TABS.filter((t) => t.tab !== 'git') : HEAD_TABS

  // Keeps scrollLeft and activeTab in sync in both directions — see
  // `use-carousel-scroll-sync.ts`'s own doc.
  const { armUserGesture, handleScroll } = useCarouselScrollSync(
    containerRef,
    activeTab,
    setActiveTab,
  )

  return (
    // Floats over the tree, never splits layout with it (spec §6): absolute
    // within ide-shell.tsx's own `relative` sidebar column, inset 8px
    // (`inset-x-2`) on the sides only — top is the resize handle below, not
    // an inset, and bottom is flush (`bottom-0`) against SidebarFooter,
    // which sits flush below this box's own positioning ancestor with no
    // gap of its own. A `bottom-2` inset here used to leave an 8px gap
    // above the footer AND, since the tree's own bottom-inset spacer
    // (`--card-bottom-inset`, measured off this card's real height) never
    // accounted for that extra 8px, let the tree's last row peek out from
    // under the card by that same amount. `bg-pane-background` is the same
    // ground `pane-container.tsx` uses; `rounded-lg` is `--radius`, not a
    // hand-rolled value.
    <div
      ref={cardRef}
      data-testid="carousel-card"
      // This box's own `absolute` already makes it a positioning ancestor,
      // so the drag-to-trash overlay below (`absolute inset-0`) covers
      // exactly this box without needing a separate `relative`.
      className="absolute inset-x-2 bottom-0 z-10 flex flex-col overflow-hidden rounded-lg border bg-pane-background"
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
        {/* The head (spec §6.1): underline variant, icon only, no labels, no
            divider. justify-start overrides the base tabs list's w-fit
            justify-center; the fold caret sits after the tabs at `ml-auto`,
            on the same head row. */}
        {/* px-1 (4px), not px-2: the row's own height (h-9) already leaves
            exactly 4px of vertical clearance around a size-7 icon-sm control
            (36 - 28 = 8, halved) — px-2 (8px) horizontally doubled that,
            reading as lopsided once this row is the WHOLE visible thing (a
            folded card, not one row among many). Matched, not guessed. */}
        <div data-testid="carousel-head" className="flex h-9 shrink-0 items-center px-1">
          {/* Balances the fold toggle's own width on the right, so the
              Files/Git group below centers on the ROW's true middle rather
              than on the leftover space next to the toggle — without it the
              icons read as shifted left, the toggle "stealing" the room it
              occupies. `size-8 sm:size-7` matches icon-sm exactly (button-
              variants.ts) so it tracks the toggle at every breakpoint. */}
          <div aria-hidden="true" className="size-8 shrink-0 sm:size-7" />
          {/* Files/Git are plain ghost Buttons, not the Tabs primitive — the
              exact same component/tokens as the fold toggle, Settings, and
              the space marks below (icon-sm, rounded-sm, text-muted-foreground,
              hover:bg-sidebar-element-hover). "Selected" reuses the space
              mark's own idiom (opacity-60 when not current) instead of a
              border/shadow/underline of its own. */}
          <div data-testid="tabs-list" className="flex flex-1 items-center justify-center gap-1">
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
                    'shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
                    !isActive && 'opacity-60',
                  )}
                >
                  <Icon size={16} weight="regular" />
                </Button>
              )
            })}
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
            className="ml-auto shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
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
        {/* The removal tray (addendum §2 step 4): "the held row renders at
            the top of the file explorer card, not at the sidebar's separate
            foot position" — moved here from `sidebar-tree-chrome.tsx`'s own
            sidebar-wide mount. Sits above the Files/Git body, independent of
            fold state: a draining hold stays visible (and Keep-able) even
            if the user folds the card while it is waiting out its clock. */}
        <RemovalTray />
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
          <SidebarCarouselFilesPanel activeWorkspaceRepoPath={activeWorkspaceRepoPath} />

          {/* Git panel */}
          <div
            data-testid="carousel-panel"
            className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
          >
            <GitPanel />
          </div>
        </div>
      </NavStack>
    </div>
  )
}
