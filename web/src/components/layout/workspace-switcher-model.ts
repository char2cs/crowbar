import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'

export interface WorkspaceSwitcherItem {
  wsId: string
  repoName: string
  branch: string
  status: WorkspaceStatus
  added?: number
  deleted?: number
  isCurrent: boolean
}

/** Flattens repos → a flat list of workspaces with repo context and current-state. */
export function flattenWorkspaces(
  repos: Repo[],
  activeWorkspaceId: string | undefined,
): WorkspaceSwitcherItem[] {
  return repos.flatMap((repo) =>
    repo.workspaces.map((ws) => ({
      wsId: ws.id,
      repoName: repo.name,
      branch: ws.branch,
      status: ws.status ?? 'new',
      added: ws.added,
      deleted: ws.deleted,
      isCurrent: ws.id === activeWorkspaceId,
    })),
  )
}
