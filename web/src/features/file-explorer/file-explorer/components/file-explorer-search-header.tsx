import { Check, Eye, Funnel, GitBranch, MagnifyingGlass as Search } from '@phosphor-icons/react'
import { useMemo, useRef, useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'
import { Button } from '@/components/ui/button'
import { Dropdown, type MenuItem } from '@/components/ui/dropdown'
import { Input } from '@/components/ui/input'
import { SidebarHeader } from '@/components/ui/sidebar'
import { cn } from '@/utils/cn'
import type { TreeSearch } from '../hooks/use-tree-search'

type FilterSetting = keyof Pick<
  Settings,
  'showHiddenFilesInFileTree' | 'showGitignoredFilesInFileTree' | 'showGitStatusInFileTree'
>

const checkMark = (on: boolean) => (on ? <Check className="size-3.5 text-primary" /> : null)

/** The tree's filter box (Enter / Shift+Enter step through matches) and its filter menu. */
export function FileExplorerSearchHeader({
  search,
  onNavigateMatch,
}: {
  search: TreeSearch
  onNavigateMatch: (direction: 1 | -1) => void
}) {
  const showHidden = useSettingsStore((s) => s.settings.showHiddenFilesInFileTree)
  const showGitignored = useSettingsStore((s) => s.settings.showGitignoredFilesInFileTree)
  const showGitStatus = useSettingsStore((s) => s.settings.showGitStatusInFileTree)
  const updateSetting = useSettingsStore((s) => s.updateSetting)
  const [isMenuOpen, setMenuOpen] = useState(false)
  const filterButtonRef = useRef<HTMLButtonElement>(null)
  const hasActiveFilters = !showHidden || !showGitignored || !showGitStatus

  const menuItems = useMemo<MenuItem[]>(() => {
    const toggle = (key: FilterSetting, value: boolean) => () => void updateSetting(key, !value)
    return [
      {
        id: 'hidden-files',
        label: 'Hidden Files',
        icon: <Eye />,
        keybinding: checkMark(showHidden),
        onClick: toggle('showHiddenFilesInFileTree', showHidden),
      },
      {
        id: 'gitignored-files',
        label: 'Gitignored Files',
        icon: <GitBranch />,
        keybinding: checkMark(showGitignored),
        onClick: toggle('showGitignoredFilesInFileTree', showGitignored),
      },
      { id: 'sep-status', label: '', separator: true, onClick: () => {} },
      {
        id: 'git-status',
        label: 'Git Status',
        icon: <GitBranch />,
        keybinding: checkMark(showGitStatus),
        onClick: toggle('showGitStatusInFileTree', showGitStatus),
      },
    ]
  }, [showGitStatus, showGitignored, showHidden, updateSetting])

  return (
    <SidebarHeader onClick={(e) => e.stopPropagation()} onMouseDown={(e) => e.stopPropagation()}>
      <div className="flex items-stretch gap-1.5">
        <span className="relative flex min-w-0 flex-1 items-center">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute start-2.5 z-10 size-3.5 text-muted-foreground/72"
          />
          <Input
            nativeInput
            ref={search.inputRef}
            value={search.query}
            onChange={(e) => search.setQuery(e.target.value)}
            size="sm"
            placeholder="Search"
            className="ps-5"
            name="file-tree-filter"
            aria-label="Filter files in tree"
            aria-controls="file-tree-results"
            autoCapitalize="none"
            autoComplete="off"
            autoCorrect="off"
            spellCheck="false"
            onKeyDown={(e) => {
              if (e.key !== 'Escape' && e.key !== 'Enter') return
              e.preventDefault()
              e.stopPropagation()
              if (e.key === 'Escape') search.close()
              else onNavigateMatch(e.shiftKey ? -1 : 1)
            }}
          />
        </span>
        <Button
          ref={filterButtonRef}
          variant="outline"
          active={hasActiveFilters}
          tooltip="Filter Files"
          tooltipSide="bottom"
          className={cn(
            'h-7.5 w-7.5 shrink-0 self-stretch rounded-lg p-0 sm:h-6.5 sm:w-6.5',
            // The theme's muted/accent tokens are ~4% alpha, so the default hover
            // just makes the button translucent over the glass sidebar. Use an
            // opaque mix of the popover base + foreground for a real muted fill.
            'hover:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_10%)] dark:hover:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_10%)]',
            'data-pressed:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_16%)] dark:data-pressed:bg-[color-mix(in_oklch,var(--popover),var(--foreground)_16%)]',
            hasActiveFilters && 'text-secondary',
          )}
          onClick={() => setMenuOpen(true)}
        >
          <Funnel className="size-3.5" />
        </Button>
      </div>
      <Dropdown
        isOpen={isMenuOpen}
        anchorRef={filterButtonRef}
        anchorSide="bottom"
        anchorAlign="end"
        items={menuItems}
        onClose={() => setMenuOpen(false)}
        closeOnSelect={false}
        className="w-fit min-w-fit"
      />
    </SidebarHeader>
  )
}
