import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'

export interface WorkspaceSwitcherItem {
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

/** Flattens repos → a flat list of workspaces with repo context and current-state. */
export function flattenWorkspaces(
  repos: Repo[],
  activeWorkspaceId: string | undefined,
): WorkspaceSwitcherItem[] {
  return repos.flatMap((repo) => {
    const items: WorkspaceSwitcherItem[] = []

    if (repo.defaultWorkspaceId) {
      items.push({
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
      items.push({
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

    return items
  })
}
