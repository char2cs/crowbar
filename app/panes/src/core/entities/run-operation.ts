import { apiFetch } from '@/lib/api/fetch'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { problemOf, type Problem } from '@/lib/errors/problem'
import {
  putLocalOperation,
  operationsState,
  subscribeOperations,
  type EntityRef,
  type Operation,
  type OperationItem,
} from '@/core/entities/operations'

/** What a request that starts an operation answers: the daemon's operation
 *  (202), or the items it created inside the request. */
export type OperationReply = { operation: Operation } | { items: OperationItem[] }

export interface OperationStart {
  kind: string
  subject: EntityRef
  projectId: string
  label: string
  items?: OperationItem[]
}

/** The header that carries an operation's client-minted id. */
export function operationHeaders(opId: string): Record<string, string> {
  return { 'Idempotency-Key': opId }
}

function settleLocal(id: string, change: Partial<Operation>): void {
  const held = operationsState().ops[id]
  if (!held?.local) return
  putLocalOperation({ ...held, ...change, updatedAt: new Date().toISOString() })
}

/** Records the operation locally at once, then sends it under its id. */
export function runOperation(
  start: OperationStart,
  send: (opId: string) => Promise<OperationReply | undefined>,
): Operation {
  const now = new Date().toISOString()
  const local: Operation = {
    ...start,
    id: crypto.randomUUID(),
    state: 'accepted',
    items: start.items ?? [],
    dismissed: false,
    version: 0,
    createdAt: now,
    updatedAt: now,
    local: true,
  }
  putLocalOperation(local)
  void send(local.id).then(
    (reply) => {
      if (reply && 'operation' in reply) putLocalOperation(reply.operation)
      else settleLocal(local.id, { state: 'succeeded', items: reply?.items ?? local.items })
    },
    (err: unknown) => settleLocal(local.id, { state: 'failed', error: problemOf(err) }),
  )
  return local
}

/** The kind of entity an operation of `kind` creates, whose arrival it waits for. */
function failureOf(op: Operation): Problem {
  return (
    op.error ??
    op.items.find((item) => item.error)?.error ?? {
      code: 'internal',
      message: `${op.label} failed`,
    }
  )
}

/** Resolves once the operation succeeded, which the daemon sends after the entities it
 *  wrote on the same scope; rejects with its reason when it failed. No timeout: each
 *  outcome is a fact the daemon sends. */
export function whenSettled(opId: string): Promise<Operation> {
  return new Promise((resolve, reject) => {
    const stops: (() => void)[] = []
    let done = false
    const check = () => {
      if (done) return
      const op = operationsState().ops[opId]
      if (!op) return
      if (op.state === 'failed') {
        done = true
        reject(new OperationFailedError(failureOf(op), opId))
      } else if (op.state === 'succeeded') {
        done = true
        resolve(op)
      }
      if (done) stops.forEach((stop) => stop())
    }
    stops.push(subscribeOperations(check))
    check()
  })
}

/** Clears a settled operation's reason; the daemon that streamed it records the dismissal. */
export async function dismissOperation(opId: string): Promise<void> {
  const { ops, hostOf } = operationsState()
  const held = ops[opId]
  if (held?.local) {
    putLocalOperation({ ...held, dismissed: true })
    return
  }
  const path = `/v0/operations/${opId}`
  putLocalOperation(await apiFetch<Operation>(path, { method: 'DELETE' }, undefined, hostOf[opId]))
}
