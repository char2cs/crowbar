import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { entityKey, splitHostKey } from '@/core/stream/host-scope'
import { onHostRemoved } from '@/features/hosts/stores/host-config-store'
import { commit, getDB } from '@/lib/persistence/idb'

export type Placed = 'workspace' | 'chat'
const KINDS: readonly Placed[] = ['workspace', 'chat']

/** Which host held a workspace or chat when last listed: all a cold start needs to place a pane. */
export interface PlacementRecord {
  key: string
  hostId: string
  kind: Placed
  id: string
}

const recordKey = (hostId: string, kind: Placed, id: string) => `${hostId}\u0000${kind}\u0000${id}`
const holderKey = (kind: Placed, id: string) => `${kind}\u0000${id}`

// kind+id -> the hosts that held it, as of the last read or write: the daemons' lists replace it.
let held = new Map<string, Set<string>>()
let tail: Promise<void> = Promise.resolve()

/** Runs `job` after the earlier ones; a failure is the cache's loss, never the caller's. */
function enqueue(job: () => Promise<unknown>): Promise<void> {
  tail = tail.then(job).then(
    () => undefined,
    () => undefined,
  )
  return tail
}

function remember(kind: Placed, id: string, hostId: string) {
  const k = holderKey(kind, id)
  held.set(k, (held.get(k) ?? new Set()).add(hostId))
}

function unremember(kind: Placed, id: string, hostId: string) {
  const k = holderKey(kind, id)
  held.get(k)?.delete(hostId)
  if (held.get(k)?.size === 0) held.delete(k)
}

/** Reads the stored placements, replacing what is held; an unreadable store reads as empty. */
export async function hydrateHostPlacements(): Promise<void> {
  held = new Map()
  const rows = await getDB()
    .then((db) => db.getAll('host-placements'))
    .catch(() => [])
  for (const row of rows) remember(row.kind, row.id, row.hostId)
}

/** The one host that held `id` when last listed, or null: unknown, or held by several. */
export function persistedHostOf(kind: Placed, id: string): string | null {
  const hosts = held.get(holderKey(kind, id))
  return hosts?.size === 1 ? [...hosts][0]! : null
}

interface Change {
  put: PlacementRecord[]
  del: PlacementRecord[]
}

const rec = (hostId: string, kind: Placed, id: string): PlacementRecord => ({
  key: recordKey(hostId, kind, id),
  hostId,
  kind,
  id,
})

function diff(s: EntityState, prev: EntityState): Change {
  const change: Change = { put: [], del: [] }
  for (const kind of KINDS) {
    if (s.byKind[kind] === prev.byKind[kind]) continue
    for (const key of Object.keys(s.byKind[kind])) {
      if (key in prev.byKind[kind]) continue
      const { hostId, id } = splitHostKey(key)
      if (!held.get(holderKey(kind, id))?.has(hostId)) change.put.push(rec(hostId, kind, id))
    }
    for (const key of Object.keys(prev.byKind[kind])) {
      if (key in s.byKind[kind]) continue
      const { hostId, id } = splitHostKey(key)
      change.del.push(rec(hostId, kind, id))
    }
  }
  return change
}

// A host whose list just landed answers for everything it holds: what it no longer lists goes.
function staleOf(s: EntityState, prev: EntityState): PlacementRecord[] {
  if (s.loaded === prev.loaded) return []
  const landed: string[] = []
  for (const scope of Object.keys(s.loaded)) {
    const { hostId, id } = splitHostKey(scope)
    if (id === 'projects' && !prev.loaded[scope]) landed.push(hostId)
  }
  const stale: PlacementRecord[] = []
  for (const hostId of landed) {
    for (const kind of KINDS) {
      for (const [k, hosts] of held) {
        const id = k.slice(kind.length + 1)
        if (!k.startsWith(`${kind}\u0000`) || !hosts.has(hostId)) continue
        if (!s.byKind[kind][entityKey(hostId, id)]) stale.push(rec(hostId, kind, id))
      }
    }
  }
  return stale
}

function apply({ put, del }: Change) {
  for (const r of put) remember(r.kind, r.id, r.hostId)
  for (const r of del) unremember(r.kind, r.id, r.hostId)
  if (put.length === 0 && del.length === 0) return
  void enqueue(async () => {
    const tx = (await getDB()).transaction('host-placements', 'readwrite')
    await commit(tx, () =>
      Promise.all([...put.map((r) => tx.store.put(r)), ...del.map((r) => tx.store.delete(r.key))]),
    )
  })
}

/** Writes each workspace and chat a daemon lists, and drops the ones it stops listing; returns
 *  the stop. Reads nothing: a cold start's `hydrateHostPlacements` does. */
export function startHostPlacementCache(): () => void {
  const sync = (s: EntityState, prev: EntityState) => {
    const change = diff(s, prev)
    change.del.push(...staleOf(s, prev))
    apply(change)
  }
  sync(entityStore.getState(), entityStore.getInitialState())
  return entityStore.subscribe(sync)
}

/** Deletes every placement of a host: it is no longer one this client follows. */
function forgetHostPlacements(hostId: string): Promise<void> {
  for (const [k, hosts] of held) {
    hosts.delete(hostId)
    if (hosts.size === 0) held.delete(k)
  }
  return enqueue(async () => {
    const tx = (await getDB()).transaction('host-placements', 'readwrite')
    await commit(tx, async () => {
      const keys = await tx.store.index('hostId').getAllKeys(hostId)
      await Promise.all(keys.map((k) => tx.store.delete(k)))
    })
  })
}

onHostRemoved((hostId) => void forgetHostPlacements(hostId))
