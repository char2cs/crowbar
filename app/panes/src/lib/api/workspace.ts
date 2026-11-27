import { apiFetch } from '@/lib/api/fetch'
import { operationHeaders, type OperationReply } from '@/core/entities/run-operation'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'

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

export function reparentWorkspace(
  wsId: string,
  newParentId: string,
  opId: string,
): Promise<OperationReply> {
  return worktreeOp(wsId, 'reparent', opId, { newParentId })
}

export function retryProvision(wsId: string, opId: string): Promise<OperationReply> {
  return worktreeOp(wsId, 'retry-provision', opId)
}

export function detachHolder(wsId: string, opId: string): Promise<OperationReply> {
  return worktreeOp(wsId, 'detach-holder', opId)
}
