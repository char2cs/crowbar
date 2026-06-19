import { apiFetch } from '@/lib/api'

/**
 * Reparent a workspace on the backend (§3, 202 Accepted). The new `parentId`
 * lives on the WorkspaceDTO delivered over the scoped WS broadcaster, so this
 * call no longer persists hierarchy locally — it just fires the hierarchical
 * mutation and the WS-driven cache reflects the new parent.
 */
export async function reparentWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
  newParentId: string,
): Promise<void> {
  await apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}/reparent`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ newParentId }),
  })
}
