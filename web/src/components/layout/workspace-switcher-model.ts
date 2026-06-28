import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

export type WorkspaceSwitcherItem =
  | {
      kind: 'home'
      projectId: string
      projectName: string
      isCurrent: boolean
    }
  | {
      kind: 'workspace'
      wsId: string
      projectId: string
      repoId: string
      repoName: string
      branch: string
      status: WorkspaceStatus
      /** §5 in-flight flag — renders the spinner over the status icon. */
      working?: boolean
      added?: number
      deleted?: number
      isCurrent: boolean
    }

/** Flattens repos → a flat list of workspaces with repo context and current-state.
 *  Prepends a home entry for the active project. */
export function flattenWorkspaces(
  repos: Repo[],
  activeWorkspaceId: string | undefined,
  isHomeRoute: boolean,
  activeProjectId: string | undefined,
  projects: Project[],
): WorkspaceSwitcherItem[] {
  const items: WorkspaceSwitcherItem[] = []

  const activeProject = projects.find((p) => p.id === activeProjectId)
  if (activeProject) {
    items.push({
      kind: 'home',
      projectId: activeProject.id,
      projectName: activeProject.name,
      isCurrent: isHomeRoute,
    })
  }

  return items.concat(
    repos.flatMap((repo) => {
      const wsItems: WorkspaceSwitcherItem[] = []

      if (repo.defaultWorkspaceId) {
        wsItems.push({
          kind: 'workspace',
          wsId: repo.defaultWorkspaceId,
          projectId: repo.projectId ?? '',
          repoId: repo.id,
          repoName: repo.name,
          branch: 'default',
          status: 'new',
          isCurrent: repo.defaultWorkspaceId === activeWorkspaceId,
        })
      }

      for (const ws of repo.workspaces) {
        wsItems.push({
          kind: 'workspace',
          wsId: ws.id,
          projectId: repo.projectId ?? '',
          repoId: repo.id,
          repoName: repo.name,
          branch: ws.branch,
          status: ws.status ?? 'new',
          working: ws.working,
          added: ws.added,
          deleted: ws.deleted,
          isCurrent: ws.id === activeWorkspaceId,
        })
      }

      return wsItems
    }),
  )
}
