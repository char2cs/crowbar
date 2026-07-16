import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectDataStore } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'

/**
 * Resolves the on-disk root of the current view (extracted from the terminal's
 * file-link resolver so Reveal in Finder shares one implementation): the active
 * workspace's worktree, the repo root for the default workspace, or — on the
 * project-home route, whose special workspace is not in the sidebar repo list —
 * the project's own path.
 */
export function resolveWorkspaceRootPath(): string | undefined {
  const wsId = getActiveWorkspaceId()
  if (wsId) {
    for (const repo of useSidebarStore.getState().repos) {
      const ws = repo.workspaces?.find((w) => w.id === wsId)
      if (ws) return ws.localPath ?? repo.localPath
      // The default (main-worktree) workspace is not in the workspaces array;
      // it maps to the repo root path.
      if (repo.defaultWorkspaceId === wsId) return repo.localPath
    }
  }
  // Project home route (/ide/<projectId>/home): use the project path.
  const projectId = window.location.hash.match(/\/ide\/([^/]+)\/home/)?.[1]
  if (projectId) {
    const projects = dataOf(useProjectDataStore.getState().data) ?? []
    return projects.find((p) => p.id === projectId)?.path
  }
  return undefined
}
