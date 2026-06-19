import { useMemo } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { Check } from '@phosphor-icons/react'
import { ArrowDownIcon, ArrowUpIcon, CornerDownLeftIcon } from 'lucide-react'
import {
  Command,
  CommandEmpty,
  CommandFooter,
  CommandInput,
  CommandItem,
  CommandList,
  CommandPanel,
} from '@/components/ui/command'
import { Kbd, KbdGroup } from '@/components/ui/kbd'
import { useSidebarStore } from '@/lib/store/sidebar'
import { fuzzyMatch } from '@/utils/search-match'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { formatChangeCount } from './format-change-count'
import { flattenWorkspaces, type WorkspaceSwitcherItem } from './workspace-switcher-model'

interface WorkspaceSwitcherMenuProps {
  /** Called after a workspace is selected (host closes the popover). */
  onClose: () => void
}

/**
 * Searchable command menu listing every workspace across repos. Selecting one
 * navigates the route only — the sidebar tab/content is never touched.
 *
 * Filtering is handled internally by the Command/Autocomplete primitive based on
 * the typed query, matching against each item's `itemToStringValue` string.
 */
export function WorkspaceSwitcherMenu({ onClose }: WorkspaceSwitcherMenuProps) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)

  const activeWorkspaceId = pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1]
  // Stable item identities across renders — base-ui tracks keyboard navigation
  // against item references, so recreating them each render makes the highlight jump.
  const items = useMemo(
    () => flattenWorkspaces(repos, activeWorkspaceId),
    [repos, activeWorkspaceId],
  )

  function select(item: WorkspaceSwitcherItem) {
    void navigate({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: item.projectId, repoId: item.repoId, wsId: item.wsId },
    })
    onClose()
  }

  return (
    <Command
      className="ui-font flex min-h-0 w-full flex-1 flex-col"
      items={items}
      itemToStringValue={(item) => {
        const ws = item as WorkspaceSwitcherItem
        return `${ws.repoName} / ${ws.branch}`
      }}
      filter={(item, query, itemToString) => fuzzyMatch(query, itemToString?.(item) ?? '')}
    >
      <CommandInput placeholder="Switch workspace…" />
      <CommandPanel className="flex min-h-0 flex-1 flex-col">
        <CommandEmpty>No workspaces found</CommandEmpty>
        <CommandList>
          {(item: WorkspaceSwitcherItem) => (
            <CommandItem
              key={item.wsId}
              className="flex items-center gap-2 font-editor"
              onClick={() => select(item)}
              value={item}
            >
              <WorkspaceBranchIcon status={item.status} />
              <span className="min-w-0 flex-1 truncate text-[13px]">
                <span className="text-muted-foreground">{item.repoName} / </span>
                <span className="text-foreground">{item.branch}</span>
              </span>
              {(item.added ?? 0) > 0 && (
                <span className="shrink-0 text-green-300">+{formatChangeCount(item.added ?? 0)}</span>
              )}
              {(item.deleted ?? 0) > 0 && (
                <span className="shrink-0 text-red-300">-{formatChangeCount(item.deleted ?? 0)}</span>
              )}
              {item.isCurrent && (
                <Check aria-label="current" className="shrink-0 text-muted-foreground" />
              )}
            </CommandItem>
          )}
        </CommandList>
      </CommandPanel>
      <CommandFooter>
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <KbdGroup>
              <Kbd>
                <ArrowUpIcon />
              </Kbd>
              <Kbd>
                <ArrowDownIcon />
              </Kbd>
            </KbdGroup>
            <span>Navigate</span>
          </div>
          <div className="flex items-center gap-2">
            <Kbd>
              <CornerDownLeftIcon />
            </Kbd>
            <span>Open</span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Kbd>Esc</Kbd>
          <span>Close</span>
        </div>
      </CommandFooter>
    </Command>
  )
}
