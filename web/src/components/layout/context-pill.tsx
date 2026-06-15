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
        className="h-8 w-full justify-start gap-2 rounded-[10px] bg-foreground/4 px-3 text-[13px] font-normal hover:bg-foreground/8"
      >
        {model.kind === 'workspace' ? (
          <span className="flex min-w-0 items-center gap-2">
            <WorkspaceBranchIcon status={model.status} />
            <span className="truncate">
              <span className="text-muted-foreground">{model.repoName}</span>
              <span className="text-muted-foreground">/</span>
              <span className="font-semibold text-foreground">{model.branchName}</span>
            </span>
          </span>
        ) : (
          <span className="truncate text-foreground">{model.projectName}</span>
        )}
      </Button>
    </div>
  )
}
