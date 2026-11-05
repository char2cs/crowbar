import { useEffect, useMemo, useRef } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { Check } from '@phosphor-icons/react'
import {
  ArrowDownIcon,
  ArrowElbowDownLeftIcon,
  ArrowUpIcon,
  BooksIcon,
} from '@phosphor-icons/react'
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
import { selectRepos, useSelect, selectProjects } from '@/core/entities/select'
import { useProjectStore } from '@/core/entities/active-project'
import { fuzzyMatch } from '@/utils/search-match'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { RepoIconMark } from './repo-icon-mark'
import { formatChangeCount } from './format-change-count'
import { flattenWorkspaces, type WorkspaceSwitcherItem } from './workspace-switcher-model'

interface WorkspaceSwitcherMenuProps {
  /** Called after a workspace is selected (host closes the popover). */
  onClose: () => void
}

/** Searchable menu of every workspace across repos. Selecting navigates the route only; the
 *  primitive filters by each item's `itemToStringValue`. */
export function WorkspaceSwitcherMenu({ onClose }: WorkspaceSwitcherMenuProps) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSelect(selectRepos)
  const projects = useSelect(selectProjects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)

  const activeWorkspaceId = pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1]
  const isHomeRoute = !activeWorkspaceId && /\/ide\/[^/]+\/home$/.test(pathname)

  // Stable item identities across renders — base-ui tracks keyboard navigation
  // against item references, so recreating them each render makes the highlight jump.
  const items = useMemo(
    () => flattenWorkspaces(repos, activeWorkspaceId, isHomeRoute, activeProjectId, projects),
    [repos, activeWorkspaceId, isHomeRoute, activeProjectId, projects],
  )

  // base-ui has no controlled highlight and settles its initial one asynchronously, so a counted
  // key sequence lands off-by-one. Instead, step its own ArrowDown one per frame until the current
  // workspace's row carries `data-highlighted`; list order is untouched.
  const rootRef = useRef<HTMLDivElement>(null)
  // Latches only once the cursor has actually landed — set at start it would be
  // cleared by StrictMode's double-invoke (first run schedules the frame, its cleanup
  // cancels it, and a start-latch would then block the real second run).
  const positionedRef = useRef(false)
  useEffect(() => {
    if (positionedRef.current) return
    const root = rootRef.current
    if (!root) return
    const currentIndex = items.findIndex((item) => item.isCurrent)
    if (currentIndex < 0) return // list not populated yet (async project load)
    if (currentIndex === 0) {
      positionedRef.current = true // already the auto-highlighted first row
      return
    }

    let frame = 0
    let steps = 0
    const maxSteps = items.length + 1 // safety bound against an unreachable target
    const step = () => {
      const currentEl = root.querySelector('[data-current="true"]')
      const input = root.querySelector('input')
      if (!currentEl || !input || steps >= maxSteps) {
        positionedRef.current = true
        return
      }
      if (currentEl.hasAttribute('data-highlighted')) {
        positionedRef.current = true // cursor arrived
        return
      }
      steps += 1
      input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
      frame = requestAnimationFrame(step) // re-check after base-ui re-renders
    }
    frame = requestAnimationFrame(step)
    return () => cancelAnimationFrame(frame)
  }, [items])

  function select(item: WorkspaceSwitcherItem) {
    if (item.kind === 'home') {
      void navigate({ to: '/ide/$projectId/home', params: { projectId: item.projectId } })
    } else {
      void navigate({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: item.projectId, repoId: item.repoId, wsId: item.wsId },
      })
    }
    onClose()
  }

  return (
    <div ref={rootRef} className="contents">
      <Command
        className="ui-font flex min-h-0 w-full flex-1 flex-col"
        items={items}
        itemToStringValue={(item) => {
          const ws = item as WorkspaceSwitcherItem
          if (ws.kind === 'home') return `${ws.projectName} home`
          return `${ws.repoName} / ${ws.branch}`
        }}
        filter={(item, query, itemToString) => fuzzyMatch(query, itemToString?.(item) ?? '')}
      >
        <CommandInput placeholder="Switch workspace…" />
        <CommandPanel className="flex min-h-0 flex-1 flex-col">
          <CommandEmpty>No workspaces found</CommandEmpty>
          <CommandList>
            {(item: WorkspaceSwitcherItem) =>
              item.kind === 'home' ? (
                <CommandItem
                  key={`home-${item.projectId}`}
                  className="flex items-center gap-2 font-editor"
                  data-current={item.isCurrent ? 'true' : undefined}
                  onClick={() => select(item)}
                  value={item}
                >
                  {/* Same mark the sidebar row and context pill use for a project
                      home. Outline regardless of selection — the trailing Check
                      already says "current", so a filled variant only doubled it. */}
                  <BooksIcon size={14} className="shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate text-[13px]">
                    <span className="text-muted-foreground">{item.projectName} / </span>
                    <span className="text-foreground">home</span>
                  </span>
                  {item.isCurrent && (
                    <Check aria-label="current" className="shrink-0 text-muted-foreground" />
                  )}
                </CommandItem>
              ) : (
                <CommandItem
                  key={item.wsId}
                  className="flex items-center gap-2 font-editor"
                  data-current={item.isCurrent ? 'true' : undefined}
                  onClick={() => select(item)}
                  value={item}
                >
                  {/* Spinner beats the avatar: a repo-home row with a working agent
                      must show its loading state, not its repo icon. */}
                  {item.repoAvatar && !item.working ? (
                    <RepoIconMark repo={item.repoAvatar} size="sm" />
                  ) : (
                    <WorkspaceBranchIcon status={item.status} working={item.working} />
                  )}
                  <span className="min-w-0 flex-1 truncate text-[13px]">
                    <span className="text-muted-foreground">{item.repoName} / </span>
                    <span className="text-foreground">{item.branch}</span>
                  </span>
                  {item.status !== 'locked' && (item.added ?? 0) > 0 && (
                    <span className="shrink-0 text-green-300">
                      +{formatChangeCount(item.added ?? 0)}
                    </span>
                  )}
                  {item.status !== 'locked' && (item.deleted ?? 0) > 0 && (
                    <span className="shrink-0 text-red-300">
                      -{formatChangeCount(item.deleted ?? 0)}
                    </span>
                  )}
                  {item.isCurrent && (
                    <Check aria-label="current" className="shrink-0 text-muted-foreground" />
                  )}
                </CommandItem>
              )
            }
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
                <ArrowElbowDownLeftIcon />
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
    </div>
  )
}
