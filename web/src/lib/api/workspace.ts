import { apiFetch } from '@/lib/api'
import { saveWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'

/**
 * Reparent a workspace via the backend, then apply the change locally.
 *
 * TODO(reparent-to-root): the backend's POST /v0/workspaces/:wsId/reparent
 * requires a non-empty newParentId, so moving a workspace back to the repo
 * root (newParentId === undefined) is applied locally only until the API
 * supports it.
 */
export async function reparentWorkspace(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  if (newParentId !== undefined) {
    await apiFetch(`/v0/workspaces/${wsId}/reparent`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ newParentId }),
    })
  }
  await handleWorkspaceReparented(wsId, newParentId, repoId)
}

/**
 * Notification handler — called after a successful reparent call and, in
 * future, by the real-time WS/SSE message from the backend.
 */
export async function handleWorkspaceReparented(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  useSidebarStore.getState().reparentWorkspace(wsId, newParentId)

  const repo = useSidebarStore.getState().repos.find((r) => r.id === repoId)
  if (!repo) return

  const entries = repo.workspaces.map((w) => ({
    wsId: w.id,
    ...(w.parentId !== undefined && { parentId: w.parentId }),
  }))

  await saveWorkspaceHierarchy(repoId, entries)
}
