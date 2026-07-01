import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'

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

/**
 * User-initiated "finish the move": rebase a moved-but-conflicting workspace onto
 * its current parent (§3, 202 Accepted). A clean rebase integrates it; a
 * conflicting one is kept for the standard resolve flow. The outcome rides the WS
 * broadcast.
 */
export async function rebaseOntoParent(wsId: string): Promise<void> {
  await apiFetch(`${workspaceBase(wsId)}/rebase-onto-parent`, { method: 'POST' })
}

/**
 * Retry provisioning a placeholder workspace in place (§3.3, 202 Accepted). On
 * success the backend attaches the worktree and the WS broadcast reflects the
 * now-managed row; a failure (branch still held) surfaces as LastError.
 */
export async function retryProvision(wsId: string): Promise<void> {
  await apiFetch(`${workspaceBase(wsId)}/retry-provision`, { method: 'POST' })
}

/**
 * Detach the holder off a placeholder's branch with the user's consent, then
 * re-provision in place (§3.5/§3.7, 202 Accepted). The outcome rides the WS
 * broadcast; a detach blocked mid-operation surfaces as LastError.
 */
export async function detachHolder(wsId: string): Promise<void> {
  await apiFetch(`${workspaceBase(wsId)}/detach-holder`, { method: 'POST' })
}
