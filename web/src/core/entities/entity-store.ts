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

/** How many connections hold a chat's scope on its host, the viewing client's own among them. */
interface PresenceDTO {
  viewers: number
}

/** The kinds this store holds, each as the daemon's list routes serve it. */
interface EntityData {
  operation: Operation
  review_thread: ThreadDTO
  /** `hostId` is the socket's that carried it, whatever the payload said. */
  project: Project
  repo: RepoDTO
  workspace: WorkspaceDTO
  chat: ChatEntity
  folder: FolderDTO
  /** A chat's newest usage report, keyed by chat id. */
  telemetry: AgentTelemetry
  /** A chat's viewers, keyed by chat id. */
  presence: PresenceDTO
}
export type StoredKind = keyof EntityData

/** Every record is filed under `entityKey(host, id)` and every scope under
 *  `hostScope(host, scope)`: the local daemon's bare, a remote host's prefixed. */
export interface EntityState {
  /** host → the daemon epoch its versions are of. */
  epochs: Record<string, string>
  byKind: { [K in StoredKind]: Record<string, Versioned<EntityData[K]>> }
  /** scope → "kind:key" it carries. */
  members: Record<string, Record<string, true>>
  /** scope → "kind:key" seen since its latest sub, until its synced. */
  fresh: Record<string, Record<string, true>>
  synced: Record<string, true>
  /** Scopes whose snapshot has landed once since boot: a resubscribe keeps what they hold. */
  loaded: Record<string, true>
  /** scope → repos its latest snapshot could not read, so it said nothing of their rows. */
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

// A frame met before its scope's synced is a snapshot row, not a live change.
function payload(hostId: string, f: EntityFrame, snapshot: boolean): unknown {
  if (f.kind === 'project') return { ...(f.data as object), hostId }
  return f.kind === 'operation' && snapshot ? { ...(f.data as Operation), snapshot: true } : f.data
}

/** A frame, with the host whose socket carried it. */
interface HeardFrame {
  hostId: string
  f: EntityFrame
}

// One entity frame over `s`, filed under its socket's host whatever its payload says:
// a newer version replaces, an older one is ignored, an equal one keeps the stored object.
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

/** Applies one entity frame `hostId`'s socket carried: a newer version replaces, an older
 *  one is ignored, an equal one keeps the stored object. */
export function applyEntityFrame(hostId: string, f: EntityFrame): void {
  if (!isStored(f.kind)) return
  entityStore.setState((s) => withFrame(s, { hostId, f }))
  settleOperations([{ hostId, f }])
}

// A scope's snapshot rows wait for its synced and land in one write with it: one
// store update, and one render, for a snapshot of any size.
const snapshotRows = new Map<string, HeardFrame[]>()

function heardEntity(hostId: string, f: EntityFrame): void {
  const rows = snapshotRows.get(hostScope(hostId, f.scope))
  if (rows) rows.push({ hostId, f })
  else applyEntityFrame(hostId, f)
}

/** A sub for `bare` went out on `hostId`: what its snapshot leaves out is pruned at synced. */
function beginSnapshot(hostId: string, bare: string): void {
  const scope = hostScope(hostId, bare)
  snapshotRows.set(scope, [])
  entityStore.setState((s) => ({
    fresh: { ...s.fresh, [scope]: {} },
    synced: without(s.synced, scope),
  }))
}

// A row the snapshot could not read stays: it did not say the row is gone. `unread` names
// repos, and projects whose home rows (no repo) it could not read.
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

/** The scope's snapshot is complete: its rows land, and an entity it no longer carries goes,
 *  unless it is a row of a repo or home the snapshot could not read. */
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

/** Each kind's records, each mapped; `undefined` drops it. */
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

/** A new epoch of `hostId`'s daemon: every version held of it is void, so any version
 *  replaces it; other hosts' versions stand. */
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

/** `hostId` was removed: every row, scope and epoch of it goes, nothing else. */
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
      forgetUnstreamedReplies((id) => id in entityStore.getState().byKind.operation)
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

/** `hostId`'s record of `kind` whose raw id is `id`. */
export function entityOf<K extends StoredKind>(
  s: EntityState,
  kind: K,
  hostId: string,
  id: string,
): EntityData[K] | undefined {
  return (s.byKind[kind] as Record<string, Versioned<EntityData[K]>>)[entityKey(hostId, id)]?.data
}

/** Writes `patch` over the held records of `kind`, by entity key, without moving their
 *  versions, for a change the daemon has accepted but not yet sent; the returned revert
 *  restores each record no frame has replaced since. */
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

/** Writes whole rows, by entity key, a write's answer carries: a held row keeps its version,
 *  and one no frame has carried yet waits at version 0, so the first frame of it replaces it. */
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

/** Drops rows, by entity key, a write's answer says are gone, before their frames land. */
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

/** Whether the store holds `ref`'s row, its id an entity key. */
export function holdsEntity(s: EntityState, ref: EntityRef): boolean {
  return ref.id in s.byKind[ref.kind]
}
