import { apiFetch } from '@/lib/api/fetch'
import { operationHeaders, type OperationReply } from '@/core/entities/run-operation'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'

// Worktree lifecycle verbs, addressed to the chat holding the worktree (spec §4.3): each answers
// 202 with the operation under `opId` (outcome on the operations stream, entity on the workspace
// one). Async so a scope not recorded yet rejects rather than throws.
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

// Reparent a workspace. `newParentId` is a workspace id — fork lineage, a different edge from the
// chat tree's ParentID; only the addressed row moved onto a chat id.
export function reparentWorkspace(
  wsId: string,
  newParentId: string,
  opId: string,
): Promise<OperationReply> {
  return worktreeOp(wsId, 'reparent', opId, { newParentId })
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
