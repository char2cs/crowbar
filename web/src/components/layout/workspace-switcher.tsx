import { useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { Check } from '@phosphor-icons/react'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { formatChangeCount } from './format-change-count'
import { flattenWorkspaces, filterWorkspaces } from './workspace-switcher-model'

interface WorkspaceSwitcherMenuProps {
  /** Called after a workspace is selected (host closes the popover). */
  onClose: () => void
}

/**
 * Searchable command menu listing every workspace across repos. Selecting one
 * navigates the route only — the sidebar tab/content is never touched.
 */
export function WorkspaceSwitcherMenu({ onClose }: WorkspaceSwitcherMenuProps) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const [query, setQuery] = useState('')

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const items = filterWorkspaces(flattenWorkspaces(repos, activeWorkspaceId), query)

  function select(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
    onClose()
  }

  return (
    <Command className="w-full">
      <CommandInput
        value={query}
        onChange={(event: React.ChangeEvent<HTMLInputElement>) => setQuery(event.target.value)}
        placeholder="Switch workspace…"
      />
      <CommandList>
        {items.length === 0 ? (
          <CommandEmpty>No workspaces found</CommandEmpty>
        ) : (
          items.map((item) => (
            <CommandItem
              key={item.wsId}
              onClick={() => select(item.wsId)}
              className="flex items-center gap-2 px-3 py-1.5 font-mono"
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
          ))
        )}
      </CommandList>
    </Command>
  )
}
