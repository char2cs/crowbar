import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { CommandDialog, CommandDialogTrigger, CommandDialogPopup } from '@/components/ui/command'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { deriveContextPillModel } from './context-pill-model'
import { WorkspaceSwitcherMenu } from './workspace-switcher'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'

/**
 * "You are here" pill above the sidebar tab bar: shows the current
 * workspace (status icon + reponame/branchname) or the active project name.
 * Clicking it opens a centered command dialog with the workspace switcher.
 */
export function ContextPill() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const [open, setOpen] = useState(false)

  const activeWorkspaceId = parseWorkspaceScopeFromPath(pathname)?.wsId
  const model = deriveContextPillModel({ activeWorkspaceId, repos, projects, activeProjectId })

  if (model.kind === 'empty') return null

  return (
    <div className="shrink-0 px-2 pt-0 pb-1">
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandDialogTrigger
          render={(
            <Button
              variant="ghost"
              aria-label="Switch workspace"
              className="h-auto w-full justify-start gap-2 rounded-lg bg-foreground/4 px-3 py-1.5 font-mono font-normal hover:bg-foreground/8 sm:h-auto"
            />
          )}
        >
          {model.kind === 'workspace' ? (
            <span className="flex w-full min-w-0 items-center gap-2">
              <span className="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left leading-tight">
                <span className="truncate text-xs text-muted-foreground">{model.repoName}</span>
                <span className="truncate text-[13px] font-semibold text-foreground">
                  {model.branchName}
                </span>
              </span>
              <span className="flex shrink-0 scale-110">
                <WorkspaceBranchIcon status={model.status} working={model.working} />
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
