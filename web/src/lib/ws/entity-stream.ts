import { wsManager } from './manager'
import {
  upsertEntity,
  removeEntity,
  getAllEntities,
  type EntityStoreName,
} from '@/lib/persistence/entity-cache'
import { isReconnectSentinel, type EntityFrame } from './types'

// §6 live-cache core: subscribe a hierarchical WS endpoint and MERGE each
// complete-DTO frame into the entity cache (status:'deleted' tombstone →
// remove), rather than refetching the whole list on every delta.
//
// Startup: run seed() (a GET) and upsert all results, so the cache is correct
// before the first push arrives. On the reconnect sentinel ({reconnected:true})
// re-run seed() — a full GET reseed — because frames missed during the outage
// can never be recovered from a single DTO merge (§6 / Open-Q4).
export interface SubscribeEntityStreamOptions<T> {
  /** Hierarchical WS endpoint, e.g. '/v0/projects/:p/repos/:r/workspaces'. */
  endpoint: string
  /** The entity object store the DTOs are keyed into. */
  store: EntityStoreName
  /** One-time GET that returns the full current set for this scope. */
  seed: () => Promise<T[]>
  /** Called after every cache mutation (seed batch, upsert, remove). */
  onChange?: () => void
}

export function subscribeEntityStream<T extends { id: string; status?: string }>(
  opts: SubscribeEntityStreamOptions<T>,
): () => void {
  const { endpoint, store, seed, onChange } = opts
  let disposed = false

  // §6 ordering: every cache mutation (seed + each live frame) is queued onto a
  // single serial promise chain so writes commit in ARRIVAL ORDER. Without this
  // each frame ran its own fire-and-forget async IDB transaction, so a 'new'
  // then 'deleted' pair for the same id could race — if the delete committed
  // first, the late upsert resurrected a tombstoned ghost row (H21). Chaining a
  // later delete strictly after an earlier upsert removes that window.
  let applyChain: Promise<void> = Promise.resolve()
  // Each reseed bumps the generation; an in-flight seed whose generation is no
  // longer current has been superseded by a newer reconnect reseed and must
  // not write stale rows.
  let seedGeneration = 0

  function applyFrame(frame: EntityFrame): Promise<void> {
    if (frame.status === 'deleted') {
      return removeEntity(store, frame.id)
    }
    return upsertEntity(store, frame as unknown as T)
  }

  // Authoritative + atomic reseed: a GET is only a point-in-time snapshot, so
  // upserting its rows is not enough — entities deleted during the outage would
  // linger. We diff the fresh set against the cache and removeEntity any id no
  // longer present, then upsert the fresh rows, pruning ghosts left by missed
  // delete frames. Folding this into applyChain (below) keeps it from
  // interleaving with live frames.
  async function applySeed(generation: number): Promise<void> {
    const items = await seed()
    // A newer reseed started while our GET was in flight; let it win.
    if (disposed || generation !== seedGeneration) return
    const fresh = new Set(items.map((item) => item.id))
    const cached = await getAllEntities<{ id: string }>(store)
    if (disposed || generation !== seedGeneration) return
    for (const entity of cached) {
      if (!fresh.has(entity.id)) await removeEntity(store, entity.id)
    }
    for (const item of items) {
      await upsertEntity(store, item)
    }
  }

  function runSeed(): void {
    const generation = ++seedGeneration
    applyChain = applyChain
      .then(() => applySeed(generation))
      .then(() => {
        if (!disposed) onChange?.()
      })
  }

  // Seed before the first push lands; the WS subscription is registered
  // synchronously below so no frame is dropped while the GET is in flight.
  runSeed()

  const unsubscribe = wsManager.subscribe(endpoint, (data: unknown) => {
    if (disposed) return
    // The reconnect sentinel is NOT a DTO — never upsert it; trigger a full
    // GET reseed so any frames missed during the outage are recovered.
    if (isReconnectSentinel(data)) {
      runSeed()
      return
    }
    const frame = data as EntityFrame
    if (!frame || typeof frame.id !== 'string') return
    applyChain = applyChain
      .then(() => applyFrame(frame))
      .then(() => {
        if (!disposed) onChange?.()
      })
  })

  return () => {
    disposed = true
    unsubscribe()
  }
}
