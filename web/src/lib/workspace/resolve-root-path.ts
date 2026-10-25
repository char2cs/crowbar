import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectDataStore } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { getWorkspaceScope } from '@/lib/workspace-scope'

/**
 * Resolves the on-disk root of workspace `wsId` (extracted from the terminal's
 * file-link resolver so Reveal in Finder shares one implementation): its worktree,
 * the repo root for the default workspace, or — for a project-home workspace, which
 * is not in the sidebar repo list — the project's own path.
 */
export function resolveWorkspaceRootPath(wsId: string): string | undefined {
  for (const repo of useSidebarStore.getState().repos) {
    const ws = repo.workspaces?.find((w) => w.id === wsId)
    if (ws) return ws.localPath ?? repo.localPath
    // The default (main-worktree) workspace is not in the workspaces array;
    // it maps to the repo root path.
    if (repo.defaultWorkspaceId === wsId) return repo.localPath
  }
  const scope = getWorkspaceScope(wsId)
  if (scope?.repoId !== '') return undefined
  const projects = dataOf(useProjectDataStore.getState().data) ?? []
  return projects.find((p) => p.id === scope.projectId)?.path
}
