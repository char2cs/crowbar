import { getAllEntities } from '@/lib/persistence/entity-cache'
import { buildScopedRepoTree, type RepoDTO, type WorkspaceDTO } from '@/lib/store/build-repo-tree'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'

/**
 * Rebuild the sidebar repo tree directly from the entity cache and push it into
 * the sidebar store. Unlike the workspace-list loadable-slice fetch (which
 * collapses bursts and may return an in-flight stale read), this is an immediate
 * un-collapsed read: callers that just wrote a DTO to the cache use it to
 * guarantee the new repo/workspace is in the sidebar BEFORE they navigate into
 * it, so the /ide route guard doesn't treat it as unknown and bounce.
 */
export async function syncSidebarFromCache(): Promise<void> {
  const [repos, workspaces] = await Promise.all([
    getAllEntities<RepoDTO>('crowbar_repos'),
    getAllEntities<WorkspaceDTO>('crowbar_workspaces'),
  ])
  // Scope to the active project: the entity cache is cross-project but the
  // sidebar only shows the active one (see build-repo-tree / workspace-list).
  const tree = buildScopedRepoTree(repos, workspaces, useProjectStore.getState().activeProjectId)
  useSidebarStore.getState().setRepos(tree)
}
