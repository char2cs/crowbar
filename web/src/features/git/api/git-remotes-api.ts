import { apiFetch } from '@/lib/api/fetch'
import { operationHeaders, type OperationReply } from '@/core/entities/run-operation'
import { gitBaseForWorkspace } from '@/lib/workspace-scope-url'

// Push and pull answer 202 with the operation that runs them; its outcome
// arrives on the operations stream. A refusal means the op never started.
const gitRemoteOp = (wsId: string, action: 'push' | 'pull', opId: string) =>
  apiFetch<OperationReply>(`${gitBaseForWorkspace(wsId)}/${action}`, {
    method: 'POST',
    headers: operationHeaders(opId),
  })

export const pushChanges = (wsId: string, opId: string): Promise<OperationReply> =>
  gitRemoteOp(wsId, 'push', opId)

export const pullChanges = (wsId: string, opId: string): Promise<OperationReply> =>
  gitRemoteOp(wsId, 'pull', opId)
