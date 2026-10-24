import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectDataStore } from '@/lib/store/projects'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { dataOf } from '@/lib/loadable'

function projectPath(projectId: string): string | undefined {
  return dataOf(useProjectDataStore.getState().data)?.find((p) => p.id === projectId)?.path
}

/**
 * `wsId`'s on-disk root: its worktree, the repo root for a repo's default
 * (main-worktree) workspace, or the project's own path for its home workspace.
 */
export function workspaceRootPath(wsId: string): string | undefined {
  for (const repo of useSidebarStore.getState().repos) {
    const ws = repo.workspaces?.find((w) => w.id === wsId)
    if (ws) return ws.localPath ?? repo.localPath
    if (repo.defaultWorkspaceId === wsId) return repo.localPath
  }
  const scope = getWorkspaceScope(wsId)
  return scope?.repoId === '' ? projectPath(scope.projectId) : undefined
}

/**
 * The on-disk root of the current view, for Reveal in Finder: the active
 * workspace's root or, on the project-home route, the project's own path.
 */
export function resolveWorkspaceRootPath(): string | undefined {
  const wsId = getActiveWorkspaceId()
  const root = wsId ? workspaceRootPath(wsId) : undefined
  if (root) return root
  const projectId = window.location.hash.match(/\/ide\/([^/]+)\/home/)?.[1]
  return projectId ? projectPath(projectId) : undefined
}
