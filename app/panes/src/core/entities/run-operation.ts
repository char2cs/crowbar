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

export type OperationReply = { operation: Operation } | { items: OperationItem[] }

export interface OperationStart {
  kind: string
  subject: EntityRef
  projectId: string
  label: string
  items?: OperationItem[]
}

export function operationHeaders(opId: string): Record<string, string> {
  return { 'Idempotency-Key': opId }
}

function settleLocal(id: string, change: Partial<Operation>): void {
  const held = operationsState().ops[id]
  if (!held?.local) return
  putLocalOperation({ ...held, ...change, updatedAt: new Date().toISOString() })
}

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

function failureOf(op: Operation): Problem {
  return (
    op.error ??
    op.items.find((item) => item.error)?.error ?? {
      code: 'internal',
      message: `${op.label} failed`,
    }
  )
}

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
