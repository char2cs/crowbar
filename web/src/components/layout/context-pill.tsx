import { useRouterState } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { deriveContextPillModel } from './context-pill-model'

/**
 * "You are here" pill above the sidebar tab bar: shows the current
 * workspace (status icon + reponame/branchname) or the active project name.
 * Clicking it jumps the sidebar to the Workspaces tab.
 */
export function ContextPill() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const model = deriveContextPillModel({ activeWorkspaceId, repos, projects, activeProjectId })

  if (model.kind === 'empty') return null

  return (
    <div className="shrink-0 px-2 pt-2 pb-1">
      <Button
        variant="ghost"
        aria-label="Show workspaces"
        onClick={() => useSidebarStore.getState().setActiveTab('workspaces')}
        className="h-auto w-full justify-start gap-2.5 rounded-lg bg-foreground/4 px-3.5 py-2.5 font-mono font-normal hover:bg-foreground/8 sm:h-auto"
      >
        {model.kind === 'workspace' ? (
          <span className="flex min-w-0 items-center gap-2">
            <WorkspaceBranchIcon status={model.status} />
            <span className="flex min-w-0 flex-col items-start gap-0.5 text-left leading-tight">
              <span className="truncate text-xs text-muted-foreground">{model.repoName}</span>
              <span className="truncate text-[13px] font-semibold text-foreground">
                {model.branchName}
              </span>
            </span>
          </span>
        ) : (
          <span className="truncate text-[13px] text-foreground">{model.projectName}</span>
        )}
      </Button>
    </div>
  )
}
