import { CaretDown, GearIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useUIState } from '@/features/window/stores/ui-state-store'
import type { SidebarPanelTabDescriptor } from './sidebar-panel-tabs'
import type { SidebarTab } from '@/lib/store/sidebar-ui'

interface CarouselHeadProps {
  tabs: readonly SidebarPanelTabDescriptor[]
  activeTab: SidebarTab
  onSelectTab: (tab: SidebarTab) => void
  showFold: boolean
  folded: boolean
  cardFolded: boolean
  onToggleFold: () => void
}

export function CarouselHead({
  tabs,
  activeTab,
  onSelectTab,
  showFold,
  folded,
  cardFolded,
  onToggleFold,
}: CarouselHeadProps) {
  // The head (spec §6.1): icon-only, no divider. px-1 (4px), not px-2: the row's
  // h-9 already leaves 4px of clearance around a size-7 control, so px-2 would
  // double it horizontally and read lopsided on a folded card.
  return (
    <div
      data-testid="carousel-head"
      className={cn('relative grid h-9 shrink-0 grid-cols-[1fr_auto_1fr] items-center px-1')}
    >
      {/* Files/Git are plain ghost Buttons, not the Tabs primitive — the
        exact same component/tokens as the fold toggle, Settings, and
        the space marks in the header (icon-sm, rounded-sm, text-muted-foreground,
        hover:bg-sidebar-element-hover). "Selected" reuses the space
        mark's own idiom (opacity-60 when not current) instead of a
        border/shadow/underline of its own. */}
      <div
        data-testid="tabs-list"
        className="relative z-10 col-start-2 flex items-center justify-center gap-1"
      >
        {tabs.map(({ tab, label, Icon }) => {
          const isActive = activeTab === tab
          return (
            <Button
              key={tab}
              variant="ghost"
              size="icon-sm"
              data-testid={`carousel-head-tab-${tab}`}
              aria-label={label}
              aria-pressed={isActive}
              onClick={() => onSelectTab(tab)}
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
        <Button
          variant="ghost"
          size="icon-sm"
          data-testid="settings-button"
          aria-label="Settings"
          tooltip="Settings"
          tooltipSide="bottom"
          onClick={() => useUIState.getState().openSettingsDialog()}
          className={'shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover'}
        >
          <GearIcon size={16} />
        </Button>
      </div>
      {showFold && (
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
          onClick={onToggleFold}
          className={cn(
            'relative z-10 shrink-0 justify-self-end rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
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
      )}
    </div>
  )
}
