import { CaretDown, MagnifyingGlass as Search } from '@phosphor-icons/react'
import { useState, type ReactNode } from 'react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { useSettingsStore } from '@/features/settings/store'
import { filterSettingsTabsBySearch } from '@/features/settings/lib/settings-tab-visibility'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'
import { SettingsVerticalTabs } from './settings-vertical-tabs'
import type { SettingsTabItem } from './settings-tab-items'

/** `tabs` narrowed to the ones the current settings search matches. */
function useVisibleSettingsTabs(tabs: SettingsTabItem[]): SettingsTabItem[] {
  const searchQuery = useSettingsStore((state) => state.search.query)
  const searchResults = useSettingsStore((state) => state.search.results)
  const matchingTabs = searchQuery ? new Set(searchResults.map((result) => result.tab)) : null
  return filterSettingsTabsBySearch(tabs, matchingTabs)
}

interface TabsProps {
  tabs: SettingsTabItem[]
  activeTab: SettingsTab
  onTabChange: (tab: SettingsTab) => void
}

/** The settings title: "Settings", or under 720px a dropdown of the tabs in place of the rail. */
export function SettingsTitle({ tabs, activeTab, onTabChange }: TabsProps) {
  const [open, setOpen] = useState(false)
  const visibleTabs = useVisibleSettingsTabs(tabs)
  const activeTabItem =
    visibleTabs.find((tab) => tab.id === activeTab) ??
    tabs.find((tab) => tab.id === activeTab) ??
    tabs[0]
  const ActiveTabIcon = activeTabItem.icon
  return (
    <>
      <span className="max-[720px]:hidden">Settings</span>
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger className="hidden h-7 max-w-48 min-w-0 items-center gap-1.5 rounded-md border border-border/70 bg-card/50 px-2 text-left text-foreground transition-colors hover:bg-muted max-[720px]:inline-flex">
          <ActiveTabIcon className="size-4 shrink-0 text-muted-foreground" weight="duotone" />
          <span className="truncate">{activeTabItem.label}</span>
          <CaretDown className="size-3.5 shrink-0 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="min-w-44">
          {visibleTabs.map((tab) => {
            const Icon = tab.icon
            return (
              <DropdownMenuItem
                key={tab.id}
                onClick={() => onTabChange(tab.id)}
                className={tab.id === activeTab ? 'bg-muted text-foreground' : undefined}
              >
                <Icon className="size-4" weight="duotone" />
                {tab.label}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  )
}

/** The settings search field; `className` sets its width. */
export function SettingsSearchInput({ className }: { className?: string }) {
  const searchQuery = useSettingsStore((state) => state.search.query)
  const setSearchQuery = useSettingsStore((state) => state.setSearchQuery)
  return (
    <Input
      type="text"
      id="settings-search"
      name="settings-search"
      placeholder="Search settings..."
      value={searchQuery}
      onChange={(e) => setSearchQuery(e.target.value)}
      leftIcon={Search}
      size="sm"
      className={className}
    />
  )
}

/** The tab rail (hidden under 720px, where the title's dropdown stands in) and the active tab. */
export function SettingsPanels({
  tabs,
  activeTab,
  onTabChange,
  children,
}: TabsProps & { children: ReactNode }) {
  return (
    <div className="flex h-full w-full min-w-0 overflow-hidden">
      <div className="w-52 shrink-0 max-[720px]:hidden">
        <SettingsVerticalTabs
          tabs={tabs}
          activeTab={activeTab}
          onTabChange={onTabChange}
          panelIdForTab={(tab) => `settings-panel-${tab}`}
        />
      </div>
      <div
        id={`settings-panel-${activeTab}`}
        role="tabpanel"
        aria-labelledby={`settings-tab-${activeTab}`}
        data-settings-content=""
        tabIndex={-1}
        className="min-w-0 flex-1 overflow-y-auto p-3 [--app-ui-control-font-size:var(--ui-text-sm)] [overscroll-behavior:contain] max-[720px]:p-2"
      >
        {children}
      </div>
    </div>
  )
}
