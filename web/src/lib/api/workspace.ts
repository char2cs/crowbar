import { apiFetch } from '@/lib/api/fetch'
import { operationHeaders, type OperationReply } from '@/lib/store/run-operation'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'

/**
 * The worktree lifecycle verbs, addressed to the CHAT that holds the worktree
 * (backend spec §4.3); the daemon resolves `:id` to the workspace behind it.
 * Each answers 202 with the operation that runs it under `opId`; its outcome
 * arrives on the operations stream, the entity changes on the workspace one.
 */
// Async so a scope that is not recorded yet rejects rather than throws.
async function worktreeOp(wsId: string, verb: string, opId: string, body?: unknown) {
  return apiFetch<OperationReply>(`${worktreeVerbBaseForWorkspace(wsId)}/${verb}`, {
    method: 'POST',
    headers: {
      ...operationHeaders(opId),
      ...(body !== undefined && { 'Content-Type': 'application/json' }),
    },
    ...(body !== undefined && { body: JSON.stringify(body) }),
  })
}

/**
 * Reparent a workspace. `newParentId` is still a WORKSPACE id: it names the fork
 * parent, which is workspace lineage and a different field from the chat tree's
 * own ParentID. Only the addressed row moved onto a chat id.
 */
export function reparentWorkspace(
  wsId: string,
  newParentId: string,
  opId: string,
): Promise<OperationReply> {
  return worktreeOp(wsId, 'reparent', opId, { newParentId })
}

/** "Finish the move": rebase a moved-but-conflicting workspace onto its parent;
 *  a conflicting rebase is kept for the standard resolve flow. */
export function rebaseOntoParent(wsId: string, opId: string): Promise<OperationReply> {
  return worktreeOp(wsId, 'rebase-onto-parent', opId)
}

/** Retry provisioning a placeholder workspace in place (§3.3). */
export function retryProvision(wsId: string, opId: string): Promise<OperationReply> {
  return worktreeOp(wsId, 'retry-provision', opId)
}

/** Detach the holder off a placeholder's branch with the user's consent, then
 *  re-provision in place (§3.5/§3.7). */
export function detachHolder(wsId: string, opId: string): Promise<OperationReply> {
  return worktreeOp(wsId, 'detach-holder', opId)
}
