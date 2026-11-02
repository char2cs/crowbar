import { createStore } from 'zustand'
import { registerScopeKind } from '@/core/stream/stream'
import type { Frame } from '@/core/stream/frames'
import { forgetLocalOperation, forgetUnstreamedReplies } from '@/lib/store/local-operations'
import type { EntityRef, Operation } from '@/core/entities/operations'
import type { FolderDTO, Project, RepoDTO, ThreadDTO, WorkspaceDTO } from '@/lib/types'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import type { ChatEntity } from '@/core/entities/chat-entity'

interface Versioned<T> {
  v: number
  data: T
}

/** The kinds this store holds, each as the daemon's list routes serve it. */
interface EntityData {
  operation: Operation
  review_thread: ThreadDTO
  project: Project
  repo: RepoDTO
  workspace: WorkspaceDTO
  chat: ChatEntity
  folder: FolderDTO
  /** A chat's newest usage report, keyed by chat id. */
  telemetry: AgentTelemetry
}
export type StoredKind = keyof EntityData

export interface EntityState {
  epoch: string
  byKind: { [K in StoredKind]: Record<string, Versioned<EntityData[K]>> }
  /** scope → "kind:id" it carries. */
  members: Record<string, Record<string, true>>
  /** scope → "kind:id" seen since its latest sub, until its synced. */
  fresh: Record<string, Record<string, true>>
  synced: Record<string, true>
  /** Scopes whose snapshot has landed once since boot: a resubscribe keeps what they hold. */
  loaded: Record<string, true>
}

export const entityStore = createStore<EntityState>()(() => ({
  epoch: '',
  byKind: {
    operation: {},
    review_thread: {},
    project: {},
    repo: {},
    workspace: {},
    chat: {},
    folder: {},
    telemetry: {},
  },
  members: {},
  fresh: {},
  synced: {},
  loaded: {},
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

type ByKind = EntityState['byKind']

function withoutEntity(byKind: ByKind, kind: StoredKind, id: string): ByKind {
  return { ...byKind, [kind]: without<unknown>(byKind[kind], id) } as ByKind
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
export function applyEntityFrame(f: EntityFrame): void {
  const kind = f.kind
  if (!isStored(kind)) return
  const key = `${kind}:${f.id}`
  entityStore.setState((s) => {
    const held = s.byKind[kind][f.id]
    if (f.gone) {
      if (!held) return s
      return {
        byKind: withoutEntity(s.byKind, kind, f.id),
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
    let byKind = s.byKind
    for (const key of Object.keys(s.members[scope] ?? {})) {
      if (seen[key] || Object.values(members).some((keys) => keys[key])) continue
      const [kind, id] = [key.slice(0, key.indexOf(':')), key.slice(key.indexOf(':') + 1)]
      if (isStored(kind)) byKind = withoutEntity(byKind, kind, id)
    }
    return {
      byKind,
      members,
      fresh: without(s.fresh, scope),
      synced: { ...s.synced, [scope]: true },
      loaded: s.loaded[scope] ? s.loaded : { ...s.loaded, [scope]: true },
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

registerScopeKind('ws', {
  subscribed: beginSnapshot,
  frame(f) {
    if (f.t === 'entity') applyEntityFrame(f)
    else if (f.t === 'synced') applySynced(f.scope)
  },
})

/** Writes `patch` over the held records of `kind` without moving their versions, for a
 *  change the daemon has accepted but not yet sent; the returned revert restores each
 *  record no frame has replaced since. */
export function patchEntities<K extends StoredKind>(
  kind: K,
  patches: Readonly<Record<string, Partial<EntityData[K]>>>,
): () => void {
  const before: Record<string, Versioned<EntityData[K]>> = {}
  if (Object.keys(patches).length === 0) return () => {}
  entityStore.setState((s) => {
    const held = s.byKind[kind] as Record<string, Versioned<EntityData[K]>>
    const next = { ...held }
    for (const [id, patch] of Object.entries(patches)) {
      const record = held[id]
      if (!record) continue
      before[id] = record
      next[id] = { v: record.v, data: { ...record.data, ...patch } }
    }
    return { byKind: { ...s.byKind, [kind]: next } }
  })
  return () =>
    entityStore.setState((s) => {
      const held = s.byKind[kind] as Record<string, Versioned<EntityData[K]>>
      const next = { ...held }
      for (const [id, record] of Object.entries(before)) {
        if (held[id]?.v === record.v) next[id] = record
      }
      return { byKind: { ...s.byKind, [kind]: next } }
    })
}

/** Writes whole rows a write's answer carries: a held row keeps its version, and one no
 *  frame has carried yet waits at version 0, so the first frame of it replaces it. */
export function placeEntities<K extends StoredKind>(
  kind: K,
  rows: Readonly<Record<string, EntityData[K]>>,
): void {
  if (Object.keys(rows).length === 0) return
  entityStore.setState((s) => {
    const held = s.byKind[kind] as Record<string, Versioned<EntityData[K]>>
    const next = { ...held }
    for (const [id, data] of Object.entries(rows)) next[id] = { v: held[id]?.v ?? 0, data }
    return { byKind: { ...s.byKind, [kind]: next } }
  })
}

/** Drops rows a write's answer says are gone, before their frames land. */
export function forgetEntities(kind: StoredKind, ids: readonly string[]): void {
  const held = entityStore.getState().byKind[kind]
  const gone = ids.filter((id) => id in held)
  if (gone.length === 0) return
  entityStore.setState((s) => {
    let byKind = s.byKind
    for (const id of gone) byKind = withoutEntity(byKind, kind, id)
    return { byKind }
  })
}

/** Whether the store holds `ref`'s row. */
export function holdsEntity(s: EntityState, ref: EntityRef): boolean {
  return ref.id in s.byKind[ref.kind]
}
