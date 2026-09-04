import { apiFetch } from '@/lib/api'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'

/**
 * The worktree lifecycle verbs, addressed to the CHAT that holds the worktree
 * (backend spec §4.3). Each is the same handler body its retired
 * `.../workspaces/:wsId/…` twin ran — the daemon resolves `:id` to the
 * workspace behind that chat's worktree and everything after that is unchanged
 * — so the outcome, the 202, and the WS frame that carries it are identical.
 */

/**
 * Reparent a workspace on the backend (§3, 202 Accepted). The new `parentId`
 * lives on the WorkspaceDTO delivered over the scoped WS broadcaster, so this
 * call no longer persists hierarchy locally — it just fires the hierarchical
 * mutation and the WS-driven cache reflects the new parent.
 *
 * `newParentId` is still a WORKSPACE id: it names the fork parent, which is
 * workspace lineage and a different field from the chat tree's own ParentID.
 * Only the addressed row moved onto a chat id, not the edge being written.
 */
export async function reparentWorkspace(wsId: string, newParentId: string): Promise<void> {
  await apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/reparent`, {
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
  await apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/rebase-onto-parent`, { method: 'POST' })
}

/**
 * Retry provisioning a placeholder workspace in place (§3.3, 202 Accepted). On
 * success the backend attaches the worktree and the WS broadcast reflects the
 * now-managed row; a failure (branch still held) surfaces as LastError.
 */
export async function retryProvision(wsId: string): Promise<void> {
  await apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/retry-provision`, { method: 'POST' })
}

/**
 * Detach the holder off a placeholder's branch with the user's consent, then
 * re-provision in place (§3.5/§3.7, 202 Accepted). The outcome rides the WS
 * broadcast; a detach blocked mid-operation surfaces as LastError.
 */
export async function detachHolder(wsId: string): Promise<void> {
  await apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/detach-holder`, { method: 'POST' })
}
