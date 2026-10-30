import { create } from 'zustand'
import type { Problem } from '@/lib/errors/problem'

export type OperationState = 'accepted' | 'running' | 'succeeded' | 'failed'

export interface EntityRef {
  kind: 'project' | 'repo' | 'workspace' | 'chat'
  id: string
}

export interface OperationItem {
  ref: EntityRef
  label: string
  state: OperationState
  error?: Problem
  /** Client-side placement of a local item. */
  parentId?: string
}

/** Work that outlives its request, as the daemon records it (`domain.Operation`). */
export interface Operation {
  id: string
  kind: string
  subject: EntityRef
  projectId: string
  label: string
  state: OperationState
  items: OperationItem[]
  error?: Problem
  dismissed: boolean
  version: number
  createdAt: string
  updatedAt: string
  /** Started by this client and not yet answered by the daemon. */
  local?: true
  /** Sent on connect (the stream's snapshot), not as a live change. */
  snapshot?: boolean
}

export interface OperationsState {
  ops: Record<string, Operation>
  /** Entity id → the newest operation naming it. */
  latestFor: Record<string, string>
}

export const useOperationsStore = create<OperationsState>()(() => ({ ops: {}, latestFor: {} }))

function namedIds(op: Operation): string[] {
  return [op.subject.id, ...op.items.map((item) => item.ref.id)]
}

function put(s: OperationsState, op: Operation): OperationsState {
  const latestFor = { ...s.latestFor }
  for (const id of namedIds(op)) {
    const newest = s.ops[latestFor[id]]
    // By time, not text: stamps differ in offset and fractional precision.
    if (!newest || newest.id === op.id || Date.parse(newest.createdAt) <= Date.parse(op.createdAt))
      latestFor[id] = op.id
  }
  return { ops: { ...s.ops, [op.id]: op }, latestFor }
}

/** Upserts a daemon frame; one that is not newer than what is held is ignored,
 *  except that it always replaces a local entry. Snapshot frames apply the same way. */
export function applyOperationFrame(op: Operation): void {
  useOperationsStore.setState((s) => {
    const held = s.ops[op.id]
    if (held && !held.local && op.version <= held.version) return s
    return put(s, op)
  })
}

/** Writes a local operation, unless the daemon's own row for it has arrived. */
export function putLocalOperation(op: Operation): void {
  useOperationsStore.setState((s) => {
    const held = s.ops[op.id]
    if (held && !held.local) return s
    return put(s, op)
  })
}

function failedOpFor(s: OperationsState, entityId: string): Operation | null {
  const op = s.ops[s.latestFor[entityId]]
  return op && op.state === 'failed' && !op.dismissed ? op : null
}

/** Why `op` failed for `entityId`: its item's own error, else the operation's. */
function problemIn(op: Operation, entityId: string): Problem | null {
  const item = op.items.find((i) => i.ref.id === entityId)
  if (item && op.subject.id !== entityId) return item.error ?? null
  return op.error ?? null
}

/** The reason the newest operation naming `entityId` failed, or null once the user dismissed it. */
export function selectEntityProblem(s: OperationsState, entityId: string): Problem | null {
  const op = failedOpFor(s, entityId)
  return op ? problemIn(op, entityId) : null
}

/** Every entity whose newest operation failed, keyed to that operation (shallow-stable). */
export function selectFailedOps(s: OperationsState): Record<string, Operation> {
  const failed: Record<string, Operation> = {}
  for (const entityId of Object.keys(s.latestFor)) {
    const op = failedOpFor(s, entityId)
    if (op) failed[entityId] = op
  }
  return failed
}

/** The reason of the newer failure among `entityIds` (a row names its chat and its workspace). */
export function newestProblem(
  failed: Readonly<Record<string, Operation>>,
  entityIds: readonly (string | null | undefined)[],
): Problem | null {
  let newest: { op: Operation; id: string } | null = null
  for (const id of entityIds) {
    const op = id ? failed[id] : undefined
    if (!op || !id) continue
    if (!newest || Date.parse(newest.op.createdAt) < Date.parse(op.createdAt)) newest = { op, id }
  }
  return newest ? problemIn(newest.op, newest.id) : null
}

/** Whether an operation naming `entityId` is still accepted or running. */
export function selectEntityBusy(s: OperationsState, entityId: string): boolean {
  for (const op of Object.values(s.ops)) {
    if (op.state !== 'accepted' && op.state !== 'running') continue
    if (op.subject.id === entityId || op.items.some((i) => i.ref.id === entityId)) return true
  }
  return false
}
