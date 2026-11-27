import { createStore } from 'zustand'
import { registerScopeKind } from '@/core/stream/stream'
import { entityKey, hostScope, splitHostKey } from '@/core/stream/host-scope'
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

interface PresenceDTO {
  viewers: number
}

interface EntityData {
  operation: Operation
  review_thread: ThreadDTO
  project: Project
  repo: RepoDTO
  workspace: WorkspaceDTO
  chat: ChatEntity
  folder: FolderDTO
  telemetry: AgentTelemetry
  presence: PresenceDTO
}
export type StoredKind = keyof EntityData

export interface EntityState {
  epochs: Record<string, string>
  byKind: { [K in StoredKind]: Record<string, Versioned<EntityData[K]>> }
  members: Record<string, Record<string, true>>
  fresh: Record<string, Record<string, true>>
  synced: Record<string, true>
  loaded: Record<string, true>
  unread: Record<string, readonly string[]>
}

export const entityStore = createStore<EntityState>()(() => ({
  epochs: {},
  byKind: {
    operation: {},
    review_thread: {},
    project: {},
    repo: {},
    workspace: {},
    chat: {},
    folder: {},
    telemetry: {},
    presence: {},
  },
  members: {},
  fresh: {},
  synced: {},
  loaded: {},
  unread: {},
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

function withoutEntity(byKind: ByKind, kind: StoredKind, key: string): ByKind {
  return { ...byKind, [kind]: without<unknown>(byKind[kind], key) } as ByKind
}

function mark(s: EntityState, scope: string, member: string): Partial<EntityState> {
  const members = { ...s.members, [scope]: { ...s.members[scope], [member]: true as const } }
  if (!s.fresh[scope]) return { members }
  return {
    members,
    fresh: { ...s.fresh, [scope]: { ...s.fresh[scope], [member]: true as const } },
  }
}

function unmarkEverywhere(members: EntityState['members'], key: string): EntityState['members'] {
  const next: EntityState['members'] = {}
  for (const [scope, keys] of Object.entries(members)) next[scope] = without(keys, key)
  return next
}

function payload(hostId: string, f: EntityFrame, snapshot: boolean): unknown {
  if (f.kind === 'project') return { ...(f.data as object), hostId }
  return f.kind === 'operation' && snapshot ? { ...(f.data as Operation), snapshot: true } : f.data
}

interface HeardFrame {
  hostId: string
  f: EntityFrame
}

function withFrame(s: EntityState, { hostId, f }: HeardFrame): EntityState {
  const kind = f.kind
  if (!isStored(kind)) return s
  const key = entityKey(hostId, f.id)
  const member = `${kind}:${key}`
  const scope = hostScope(hostId, f.scope)
  const held = s.byKind[kind][key]
  if (f.gone) {
    if (!held) return s
    return {
      ...s,
      byKind: withoutEntity(s.byKind, kind, key),
      members: unmarkEverywhere(s.members, member),
    }
  }
  const marked = { ...s, ...mark(s, scope, member) }
  if (held && f.v <= held.v) return marked
  const next = { v: f.v, data: payload(hostId, f, !s.synced[scope]) } as Versioned<
    EntityData[typeof kind]
  >
  return { ...marked, byKind: { ...s.byKind, [kind]: { ...s.byKind[kind], [key]: next } } }
}

function settleOperations(frames: readonly HeardFrame[]): void {
  for (const { f } of frames) {
    if (f.kind === 'operation' && !f.gone) forgetLocalOperation(f.id, (f.data as Operation).version)
  }
}

export function applyEntityFrame(hostId: string, f: EntityFrame): void {
  if (!isStored(f.kind)) return
  entityStore.setState((s) => withFrame(s, { hostId, f }))
  settleOperations([{ hostId, f }])
}

const snapshotRows = new Map<string, HeardFrame[]>()

function heardEntity(hostId: string, f: EntityFrame): void {
  const rows = snapshotRows.get(hostScope(hostId, f.scope))
  if (rows) rows.push({ hostId, f })
  else applyEntityFrame(hostId, f)
}

function beginSnapshot(hostId: string, bare: string): void {
  const scope = hostScope(hostId, bare)
  snapshotRows.set(scope, [])
  entityStore.setState((s) => ({
    fresh: { ...s.fresh, [scope]: {} },
    synced: without(s.synced, scope),
  }))
}

function inUnreadScope(byKind: ByKind, kind: StoredKind, key: string, unread: ReadonlySet<string>) {
  if (unread.size === 0 || (kind !== 'workspace' && kind !== 'chat' && kind !== 'folder')) {
    return false
  }
  const data = byKind[kind][key]?.data
  const owner = data && (data.repoId || data.projectId)
  return !!owner && unread.has(owner)
}

function splitMember(member: string): [string, string] {
  const colon = member.indexOf(':')
  return [member.slice(0, colon), member.slice(colon + 1)]
}

function applySynced(hostId: string, bare: string, unread: readonly string[] = []): void {
  const scope = hostScope(hostId, bare)
  const rows = snapshotRows.get(scope) ?? []
  snapshotRows.delete(scope)
  const unreadRepos = new Set(unread)
  if (unreadRepos.size > 0) console.warn(`${scope}: rows of repos ${unread.join(', ')} unread`)
  entityStore.setState((held) => {
    const s = rows.reduce(withFrame, held)
    const seen = { ...(s.fresh[scope] ?? s.members[scope]) }
    const members = { ...s.members, [scope]: seen }
    let byKind = s.byKind
    for (const member of Object.keys(s.members[scope] ?? {})) {
      if (seen[member] || Object.values(members).some((keys) => keys[member])) continue
      const [kind, key] = splitMember(member)
      if (!isStored(kind)) continue
      if (inUnreadScope(byKind, kind, key, unreadRepos)) seen[member] = true
      else byKind = withoutEntity(byKind, kind, key)
    }
    return {
      byKind,
      members,
      fresh: without(s.fresh, scope),
      synced: { ...s.synced, [scope]: true },
      loaded: s.loaded[scope] ? s.loaded : { ...s.loaded, [scope]: true },
      unread: { ...s.unread, [scope]: unread },
    }
  })
  settleOperations(rows)
}

const ofHost = (hostId: string, key: string) => splitHostKey(key).hostId === hostId

function mapRecords(byKind: ByKind, map: (key: string, held: Versioned<unknown>) => unknown) {
  const next: Record<string, Record<string, unknown>> = {}
  for (const [kind, records] of Object.entries(byKind)) {
    next[kind] = {}
    for (const [key, held] of Object.entries(records as Record<string, Versioned<unknown>>)) {
      const mapped = map(key, held)
      if (mapped !== undefined) next[kind][key] = mapped
    }
  }
  return next as unknown as ByKind
}

function resetForEpoch(hostId: string, epoch: string): void {
  entityStore.setState((s) => {
    if (s.epochs[hostId] === epoch) return s
    const byKind = mapRecords(s.byKind, (key, held) =>
      ofHost(hostId, key) ? { v: 0, data: held.data } : held,
    )
    return { epochs: { ...s.epochs, [hostId]: epoch }, byKind }
  })
}

function withoutHostKeys<T>(record: Record<string, T>, hostId: string): Record<string, T> {
  return Object.fromEntries(Object.entries(record).filter(([key]) => !ofHost(hostId, key)))
}

function forgetHost(hostId: string): void {
  for (const scope of [...snapshotRows.keys()]) {
    if (ofHost(hostId, scope)) snapshotRows.delete(scope)
  }
  entityStore.setState((s) => ({
    epochs: without(s.epochs, hostId),
    byKind: mapRecords(s.byKind, (key, held) => (ofHost(hostId, key) ? undefined : held)),
    members: withoutHostKeys(s.members, hostId),
    fresh: withoutHostKeys(s.fresh, hostId),
    synced: withoutHostKeys(s.synced, hostId),
    loaded: withoutHostKeys(s.loaded, hostId),
    unread: withoutHostKeys(s.unread, hostId),
  }))
}

registerScopeKind('projects', {
  subscribed: beginSnapshot,
  epoch: resetForEpoch,
  forget: forgetHost,
  frame(hostId, f) {
    if (f.t === 'entity') heardEntity(hostId, f)
    else if (f.t === 'synced') {
      applySynced(hostId, f.scope, f.unread)
      const ops = Object.keys(entityStore.getState().byKind.operation)
      const streamed = new Set(ops.map((key) => splitHostKey(key).id))
      forgetUnstreamedReplies((id) => streamed.has(id))
    }
  },
})

registerScopeKind('ws', {
  subscribed: beginSnapshot,
  frame(hostId, f) {
    if (f.t === 'entity') heardEntity(hostId, f)
    else if (f.t === 'synced') applySynced(hostId, f.scope)
  },
})

export function entityOf<K extends StoredKind>(
  s: EntityState,
  kind: K,
  hostId: string,
  id: string,
): EntityData[K] | undefined {
  return (s.byKind[kind] as Record<string, Versioned<EntityData[K]>>)[entityKey(hostId, id)]?.data
}

export function patchEntities<K extends StoredKind>(
  kind: K,
  patches: Readonly<Record<string, Partial<EntityData[K]>>>,
): () => void {
  const before: Record<string, Versioned<EntityData[K]>> = {}
  if (Object.keys(patches).length === 0) return () => {}
  entityStore.setState((s) => {
    const held = s.byKind[kind] as Record<string, Versioned<EntityData[K]>>
    const next = { ...held }
    for (const [key, patch] of Object.entries(patches)) {
      const record = held[key]
      if (!record) continue
      before[key] = record
      next[key] = { v: record.v, data: { ...record.data, ...patch } }
    }
    return { byKind: { ...s.byKind, [kind]: next } }
  })
  return () =>
    entityStore.setState((s) => {
      const held = s.byKind[kind] as Record<string, Versioned<EntityData[K]>>
      const next = { ...held }
      for (const [key, record] of Object.entries(before)) {
        if (held[key]?.v === record.v) next[key] = record
      }
      return { byKind: { ...s.byKind, [kind]: next } }
    })
}

export function placeEntities<K extends StoredKind>(
  kind: K,
  rows: Readonly<Record<string, EntityData[K]>>,
): void {
  if (Object.keys(rows).length === 0) return
  entityStore.setState((s) => {
    const held = s.byKind[kind] as Record<string, Versioned<EntityData[K]>>
    const next = { ...held }
    for (const [key, data] of Object.entries(rows)) next[key] = { v: held[key]?.v ?? 0, data }
    return { byKind: { ...s.byKind, [kind]: next } }
  })
}

export function forgetEntities(kind: StoredKind, keys: readonly string[]): void {
  const held = entityStore.getState().byKind[kind]
  const gone = keys.filter((key) => key in held)
  if (gone.length === 0) return
  entityStore.setState((s) => {
    let byKind = s.byKind
    for (const key of gone) byKind = withoutEntity(byKind, kind, key)
    return { byKind }
  })
}

export function holdsEntity(s: EntityState, ref: EntityRef): boolean {
  return ref.id in s.byKind[ref.kind]
}
