import { useCallback, useState } from 'react'
import { House } from '@phosphor-icons/react'
import { useRouterState } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { CommandDialog, CommandDialogTrigger, CommandDialogPopup } from '@/components/ui/command'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
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
  const [open, setOpen] = useState(false)
  const openSwitcher = useCallback(() => setOpen(true), [])
  useWorkspaceSwitcherKeyboard(openSwitcher)

  const activeWorkspaceId = parseWorkspaceScopeFromPath(pathname)?.wsId
  const isHomeRoute = !activeWorkspaceId && /\/ide\/[^/]+\/home$/.test(pathname)
  const model = deriveContextPillModel({ activeWorkspaceId, isHomeRoute, repos, projects, activeProjectId })

  if (model.kind === 'empty') return null

  return (
    <div className="shrink-0 px-2 pt-0 pb-1">
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandDialogTrigger
          render={(
            <Button
              variant="ghost"
              aria-label="Switch workspace"
              className="h-auto w-full justify-start gap-2 rounded-lg bg-sidebar-element-idle px-3 py-1.5 font-mono font-normal hover:bg-sidebar-element-hover sm:h-auto"
            />
          )}
        >
          {model.kind === 'workspace' ? (
            <span className="flex w-full min-w-0 items-center gap-2">
              <span className="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-tight">
                <span className="truncate text-xs text-foreground/70">{model.repoName}</span>
                <span className="truncate text-[13px] font-semibold text-foreground">
                  {model.branchName}
                </span>
              </span>
              <span className="flex shrink-0 scale-110">
                {model.repoAvatar ? (
                  <RepoAvatar avatar={model.repoAvatar} name={model.repoName} size="lg" />
                ) : (
                  <WorkspaceBranchIcon status={model.status} working={model.working} />
                )}
              </span>
            </span>
          ) : model.kind === 'home' ? (
            <span className="flex w-full min-w-0 items-center gap-2">
              <span className="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-tight">
                <span className="truncate font-mono text-xs text-foreground/70">{model.projectName}</span>
                <span className="truncate font-mono text-[13px] font-semibold text-foreground">home</span>
              </span>
              <span className="flex shrink-0 scale-110 text-foreground/70">
                <House size={14} weight="fill" />
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
