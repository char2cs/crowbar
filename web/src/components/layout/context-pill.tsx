import { useCallback, useState } from 'react'
import { Library } from 'lucide-react'
import { useRouterState } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { CommandDialog, CommandDialogTrigger, CommandDialogPopup } from '@/components/ui/command'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { dataOf } from '@/lib/loadable'
import { WorkspaceBranchIcon, WorkspaceAgentSpinner } from './workspace-branch-icon'
import { deriveContextPillModel } from './context-pill-model'
import { WorkspaceSwitcherMenu } from './workspace-switcher'
import { RepoAvatar } from './repo-avatar'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { useWorkspaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-workspace-switcher-keyboard'

/**
 * "You are here" pill above the sidebar tab bar: shows the current
 * workspace (status icon + reponame/branchname) or the active project name.
 * Clicking it opens a centered command dialog with the workspace switcher.
 */
export function ContextPill() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const homeWorking = useHomeWorkspaceStore((s) => s.workspace?.working ?? false)
  const [open, setOpen] = useState(false)
  const openSwitcher = useCallback(() => setOpen(true), [])
  useWorkspaceSwitcherKeyboard(openSwitcher)

  const activeWorkspaceId = parseWorkspaceScopeFromPath(pathname)?.wsId
  const isHomeRoute = !activeWorkspaceId && /\/ide\/[^/]+\/home$/.test(pathname)
  const model = deriveContextPillModel({
    activeWorkspaceId,
    isHomeRoute,
    repos,
    projects,
    activeProjectId,
    homeWorking,
  })

  if (model.kind === 'empty') return null

  return (
    <div className="shrink-0 px-2 pt-0 pb-1" data-oracle-id="context-pill">
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandDialogTrigger
          render={
            <Button
              variant="ghost"
              aria-label="Switch workspace"
              className="h-auto w-full justify-start gap-2 rounded-lg bg-sidebar-element-idle px-3 py-1.5 font-mono font-normal hover:bg-sidebar-element-hover sm:h-auto"
              data-oracle-id="context-pill-trigger"
            />
          }
        >
          {model.kind === 'workspace' ? (
            <span className="flex w-full min-w-0 items-center gap-2">
              {/* `w-full` on each line is load-bearing, not decoration: the column
                  is `items-start`, so a child's width is shrink-to-fit — and with
                  `white-space: nowrap` (truncate) its min-content width IS the
                  whole string, so it sizes to the full branch name and
                  `overflow: hidden` clips nothing. A long branch then painted
                  straight over the status icon. Pinning each line to the column's
                  width gives truncate something to truncate against. */}
              <span className="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-tight">
                <span className="w-full truncate text-xs text-foreground/70">{model.repoName}</span>
                <span className="w-full truncate text-[13px] font-semibold text-foreground">
                  {model.branchName}
                </span>
              </span>
              <span className="flex shrink-0 scale-110">
                {/* The repo-home pill normally shows the repo avatar, but a working
                    agent must still spin its icon — so the spinner (rendered by
                    WorkspaceBranchIcon when `working`) takes precedence over it. */}
                {model.repoAvatar && !model.working ? (
                  <RepoAvatar avatar={model.repoAvatar} name={model.repoName} size="lg" />
                ) : (
                  <WorkspaceBranchIcon status={model.status} working={model.working} />
                )}
              </span>
            </span>
          ) : model.kind === 'home' ? (
            <span className="flex w-full min-w-0 items-center gap-2">
              {/* Same `w-full`-per-line rule as the workspace pill above — a long
                  project name would otherwise run under the house icon. */}
              <span className="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-tight">
                <span className="w-full truncate font-mono text-xs text-foreground/70">
                  {model.projectName}
                </span>
                <span className="w-full truncate font-mono text-[13px] font-semibold text-foreground">
                  home
                </span>
              </span>
              <span className="flex shrink-0 scale-110 text-foreground/70">
                {/* Same mark as the home row's leading glyph, at Lucide's default
                    weight like every other Lucide icon in the sidebar. */}
                {model.working ? <WorkspaceAgentSpinner /> : <Library size={14} />}
              </span>
            </span>
          ) : (
            <span className="truncate text-[13px] text-foreground">{model.projectName}</span>
          )}
        </CommandDialogTrigger>
        <CommandDialogPopup backdropClassName="backdrop-blur-xs">
          <WorkspaceSwitcherMenu onClose={() => setOpen(false)} />
        </CommandDialogPopup>
      </CommandDialog>
    </div>
  )
}
