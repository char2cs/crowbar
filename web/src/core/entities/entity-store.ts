import { createStore } from 'zustand'
import { registerScopeKind } from '@/core/stream/stream'
import type { Frame } from '@/core/stream/frames'
import { forgetLocalOperation, forgetUnstreamedReplies } from '@/lib/store/local-operations'
import type { Operation } from '@/lib/store/operations'

interface Versioned<T> {
  v: number
  data: T
}

/** The kinds this store holds; T6 and T10 add theirs. */
interface EntityData {
  operation: Operation
}
type StoredKind = keyof EntityData

export interface EntityState {
  epoch: string
  byKind: { [K in StoredKind]: Record<string, Versioned<EntityData[K]>> }
  /** scope → "kind:id" it carries. */
  members: Record<string, Record<string, true>>
  /** scope → "kind:id" seen since its latest sub, until its synced. */
  fresh: Record<string, Record<string, true>>
  synced: Record<string, true>
}

export const entityStore = createStore<EntityState>()(() => ({
  epoch: '',
  byKind: { operation: {} },
  members: {},
  fresh: {},
  synced: {},
}))

type EntityFrame = Extract<Frame, { t: 'entity' }>

function isStored(kind: string): kind is StoredKind {
  return Object.hasOwn(entityStore.getState().byKind, kind)
}

function without<T>(record: Record<string, T>, key: string): Record<string, T> {
  const next = { ...record }
  delete next[key]
  return next
}

function mark(s: EntityState, scope: string, key: string): Partial<EntityState> {
  const members = { ...s.members, [scope]: { ...s.members[scope], [key]: true as const } }
  if (!s.fresh[scope]) return { members }
  return { members, fresh: { ...s.fresh, [scope]: { ...s.fresh[scope], [key]: true as const } } }
}

function unmarkEverywhere(members: EntityState['members'], key: string): EntityState['members'] {
  const next: EntityState['members'] = {}
  for (const [scope, keys] of Object.entries(members)) next[scope] = without(keys, key)
  return next
}

// A frame met before its scope's synced is a snapshot row, not a live change.
function payload(f: EntityFrame, snapshot: boolean): unknown {
  return f.kind === 'operation' && snapshot ? { ...(f.data as Operation), snapshot: true } : f.data
}

/** Applies one entity frame: a newer version replaces, an older one is ignored,
 *  an equal one keeps the stored object. */
function applyEntityFrame(f: EntityFrame): void {
  const kind = f.kind
  if (!isStored(kind)) return
  const key = `${kind}:${f.id}`
  entityStore.setState((s) => {
    const held = s.byKind[kind][f.id]
    if (f.gone) {
      if (!held) return s
      return {
        byKind: { ...s.byKind, [kind]: without(s.byKind[kind], f.id) },
        members: unmarkEverywhere(s.members, key),
      }
    }
    const marked = mark(s, f.scope, key)
    if (held && f.v <= held.v) return marked
    const next = { v: f.v, data: payload(f, !s.synced[f.scope]) } as Versioned<
      EntityData[typeof kind]
    >
    return { ...marked, byKind: { ...s.byKind, [kind]: { ...s.byKind[kind], [f.id]: next } } }
  })
  if (kind === 'operation' && !f.gone) forgetLocalOperation(f.id, (f.data as Operation).version)
}

/** A sub for `scope` went out: what its snapshot leaves out is pruned at synced. */
function beginSnapshot(scope: string): void {
  entityStore.setState((s) => ({
    fresh: { ...s.fresh, [scope]: {} },
    synced: without(s.synced, scope),
  }))
}

/** The scope's snapshot is complete: an entity it no longer carries goes. */
function applySynced(scope: string): void {
  entityStore.setState((s) => {
    const seen = s.fresh[scope] ?? s.members[scope] ?? {}
    const members = { ...s.members, [scope]: seen }
    const byKind = { ...s.byKind }
    for (const key of Object.keys(s.members[scope] ?? {})) {
      if (seen[key] || Object.values(members).some((keys) => keys[key])) continue
      const [kind, id] = [key.slice(0, key.indexOf(':')), key.slice(key.indexOf(':') + 1)]
      if (isStored(kind)) byKind[kind] = without(byKind[kind], id)
    }
    return {
      byKind,
      members,
      fresh: without(s.fresh, scope),
      synced: { ...s.synced, [scope]: true },
    }
  })
}

/** A new daemon epoch: every version held is void, so any version replaces it. */
function resetForEpoch(epoch: string): void {
  entityStore.setState((s) => {
    if (s.epoch === epoch) return s
    const byKind = {} as EntityState['byKind']
    for (const kind of Object.keys(s.byKind) as StoredKind[]) {
      byKind[kind] = Object.fromEntries(
        Object.entries(s.byKind[kind]).map(([id, held]) => [id, { v: 0, data: held.data }]),
      )
    }
    return { epoch, byKind }
  })
}

registerScopeKind('projects', {
  subscribed: beginSnapshot,
  epoch: resetForEpoch,
  frame(f) {
    if (f.t === 'entity') applyEntityFrame(f)
    else if (f.t === 'synced') {
      applySynced(f.scope)
      forgetUnstreamedReplies((id) => id in entityStore.getState().byKind.operation)
    }
  },
})
