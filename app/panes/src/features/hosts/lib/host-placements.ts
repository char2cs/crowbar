import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { entityKey, splitHostKey } from '@/core/stream/host-scope'
import { onHostRemoved } from '@/features/hosts/stores/host-config-store'
import { commit, getDB } from '@/lib/persistence/idb'
import type { PlacementRecord } from '@/lib/persistence/placement-record'
import { WriteQueue } from '@/lib/persistence/write-queue'

type Placed = PlacementRecord['kind']
const KINDS: readonly Placed[] = ['workspace', 'chat']
const STORE = 'host-placements'
const HOSTS_LIST = 'projects'
const SEP = '\u0000'

interface Change {
  put: PlacementRecord[]
  del: PlacementRecord[]
}

const holderKey = (kind: Placed, id: string) => `${kind}${SEP}${id}`

const recordOf = (hostId: string, kind: Placed, id: string): PlacementRecord => ({
  key: `${hostId}${SEP}${kind}${SEP}${id}`,
  hostId,
  kind,
  id,
})

function onlyIn(s: EntityState, other: EntityState, kind: Placed): PlacementRecord[] {
  return Object.keys(s.byKind[kind])
    .filter((key) => !(key in other.byKind[kind]))
    .map((key) => {
      const { hostId, id } = splitHostKey(key)
      return recordOf(hostId, kind, id)
    })
}

function landedHosts(s: EntityState, prev: EntityState): string[] {
  if (s.loaded === prev.loaded) return []
  return Object.keys(s.loaded)
    .filter((scope) => !prev.loaded[scope])
    .map(splitHostKey)
    .filter(({ id }) => id === HOSTS_LIST)
    .map(({ hostId }) => hostId)
}

async function writeChange({ put, del }: Change): Promise<void> {
  const tx = (await getDB()).transaction(STORE, 'readwrite')
  const write = () =>
    Promise.all([...put.map((r) => tx.store.put(r)), ...del.map((r) => tx.store.delete(r.key))])
  await commit(tx, write)
}

async function deleteHostRows(hostId: string): Promise<void> {
  const tx = (await getDB()).transaction(STORE, 'readwrite')
  const write = async () => {
    const keys = await tx.store.index('hostId').getAllKeys(hostId)
    await Promise.all(keys.map((k) => tx.store.delete(k)))
  }
  await commit(tx, write)
}

class Placements {
  private held = new Map<string, Set<string>>()
  private readonly queue = new WriteQueue()

  async hydrate(): Promise<void> {
    this.held = new Map()
    const rows = await getDB()
      .then((db) => db.getAll(STORE))
      .catch(() => [])
    rows.forEach((r) => this.remember(r))
  }

  hostOf(kind: Placed, id: string): string | null {
    const [only, ...more] = this.held.get(holderKey(kind, id)) ?? []
    return only !== undefined && more.length === 0 ? only : null
  }

  sync(s: EntityState, prev: EntityState): void {
    const kinds = KINDS.filter((kind) => s.byKind[kind] !== prev.byKind[kind])
    const listed = landedHosts(s, prev).flatMap((hostId) => this.heldBy(hostId))
    const change: Change = {
      put: kinds.flatMap((kind) => onlyIn(s, prev, kind)).filter((r) => !this.isHeld(r)),
      del: [
        ...kinds.flatMap((kind) => onlyIn(prev, s, kind)),
        ...listed.filter((r) => !s.byKind[r.kind][entityKey(r.hostId, r.id)]),
      ],
    }
    change.put.forEach((r) => this.remember(r))
    change.del.forEach((r) => this.unremember(r))
    if (change.put.length === 0 && change.del.length === 0) return
    void this.queue.push(() => writeChange(change))
  }

  forgetHost(hostId: string): void {
    this.heldBy(hostId).forEach((r) => this.unremember(r))
    void this.queue.push(() => deleteHostRows(hostId))
  }

  private remember({ kind, id, hostId }: PlacementRecord): void {
    const k = holderKey(kind, id)
    this.held.set(k, (this.held.get(k) ?? new Set()).add(hostId))
  }

  private unremember({ kind, id, hostId }: PlacementRecord): void {
    const k = holderKey(kind, id)
    this.held.get(k)?.delete(hostId)
    if (this.held.get(k)?.size === 0) this.held.delete(k)
  }

  private isHeld({ kind, id, hostId }: PlacementRecord): boolean {
    return this.held.get(holderKey(kind, id))?.has(hostId) === true
  }

  private heldBy(hostId: string): PlacementRecord[] {
    return [...this.held]
      .filter(([, hosts]) => hosts.has(hostId))
      .map(([k]) => {
        const at = k.indexOf(SEP)
        return recordOf(hostId, k.slice(0, at) as Placed, k.slice(at + SEP.length))
      })
  }
}

const placements = new Placements()

/** Reads the stored placements, replacing what is held; an unreadable store reads as empty. */
export function hydrateHostPlacements(): Promise<void> {
  return placements.hydrate()
}

/** The one host that held `id` when last listed, or null: unknown, or held by several. */
export function persistedHostOf(kind: Placed, id: string): string | null {
  return placements.hostOf(kind, id)
}

/** Writes each workspace and chat a daemon lists, and drops the ones it stops listing (a host
 *  whose list lands answers for everything it held); returns the stop. Reads nothing: a cold
 *  start's `hydrateHostPlacements` does. A removed host's placements go with it. */
export function startHostPlacementCache(): () => void {
  const sync = (s: EntityState, prev: EntityState) => placements.sync(s, prev)
  sync(entityStore.getState(), entityStore.getInitialState())
  return entityStore.subscribe(sync)
}

onHostRemoved((hostId) => placements.forgetHost(hostId))
