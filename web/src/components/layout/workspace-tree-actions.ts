import { useSidebarStore } from '@/lib/store/sidebar'
import { postWorkspace, deleteWorkspace as apiDeleteWorkspace } from '@/lib/api'

/**
 * Resolve the owning project id for a repo from the sidebar tree. Hierarchical
 * mutations need both ids; the tree always carries `projectId` from the §5
 * RepoDTO once the repo has seeded.
 */
export function projectIdForRepo(repoId: string): string | undefined {
  return useSidebarStore.getState().repos.find((r) => r.id === repoId)?.projectId
}

/**
 * Fire the hierarchical create mutation (202 Accepted, §3). No optimistic node
 * is added: the WorkspaceDTO arrives over the scoped WS stream (status 'new'
 * then the real status) and the WS-driven cache inserts it.
 */
export async function performCreateWorkspace(
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<void> {
  const projectId = projectIdForRepo(repoId)
  if (!projectId) {
    console.error('Failed to create workspace: unknown project for repo', repoId)
    return
  }
  try {
    await postWorkspace(projectId, repoId, branch, parentId)
  } catch (err) {
    console.error('Failed to create workspace:', err)
  }
}

/**
 * Fire the hierarchical delete mutation (202 Accepted, §3). Locked workspaces
 * are never deleted. No optimistic removal: the backend owns the cascade and
 * emits one status:'deleted' tombstone per removed id, which the WS-driven
 * cache applies. On failure the item stays in the list via WS non-arrival — no
 * toast needed.
 */
export async function performDeleteWorkspace(wsId: string): Promise<void> {
  const repo = useSidebarStore.getState().repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const ws = repo?.workspaces.find((w) => w.id === wsId)
  if (!repo || !ws || ws.status === 'locked') return
  const projectId = repo.projectId
  if (!projectId) return
  try {
    await apiDeleteWorkspace(projectId, repo.id, wsId)
  } catch (err) {
    console.error('Failed to delete workspace:', err)
    // item stays in list via WS non-arrival — no toast needed
  }
}
