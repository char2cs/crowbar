import { readSelect, selectRepos } from '@/core/entities/select'
import { entityOf, entityStore } from '@/core/entities/entity-store'
import { resolvedHostOf } from '@/lib/host-resolve'
import { getWorkspaceScope } from '@/lib/workspace-scope'

function projectPath(projectId: string): string | undefined {
  const hostId = resolvedHostOf({ projectId })
  return hostId ? entityOf(entityStore.getState(), 'project', hostId, projectId)?.path : undefined
}

/**
 * `wsId`'s on-disk root: its worktree, the repo root for a repo's default
 * (main-worktree) workspace, or the project's own path for its home workspace.
 */
export function workspaceRootPath(wsId: string): string | undefined {
  for (const repo of readSelect(selectRepos)) {
    const ws = repo.workspaces?.find((w) => w.id === wsId)
    if (ws) return ws.localPath ?? repo.localPath
    if (repo.defaultWorkspaceId === wsId) return repo.localPath
  }
  const scope = getWorkspaceScope(wsId)
  return scope?.repoId === '' ? projectPath(scope.projectId) : undefined
}
