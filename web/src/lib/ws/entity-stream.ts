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
  /**
   * Predicate marking which CACHED entities this seed is authoritative over —
   * only matching entities absent from the fresh seed are pruned as ghosts.
   * Omit when the seed owns the entire store.
   *
   * REQUIRED whenever multiple streams share ONE store partitioned by scope.
   * `crowbar_workspaces` is fed by BOTH the per-repo LIST stream (seed = all of
   * a repo's workspaces) AND the per-:wsId stream (seed = just the viewed
   * workspace). Without a scope, the single-workspace seed is treated as
   * authoritative over the whole store and prunes every sibling — navigating
   * into one workspace deleted all the others from the sidebar until reload.
   * Each stream sets `pruneScope` to its own scope: `(ws) => ws.id === wsId` for
   * the single-workspace stream, `(ws) => ws.repoId === repoId` for the
   * per-repo list, `(repo) => repo.projectId === projectId` for the repo list.
   */
  pruneScope?: (cached: T) => boolean
}

export function subscribeEntityStream<T extends { id: string; status?: string }>(
  opts: SubscribeEntityStreamOptions<T>,
): () => void {
  const { endpoint, store, seed, onChange, pruneScope } = opts
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
    const cached = await getAllEntities<T>(store)
    if (disposed || generation !== seedGeneration) return
    for (const entity of cached) {
      // Prune a ghost ONLY if this seed is authoritative over it. Without a
      // pruneScope the seed owns the whole store (legacy behaviour). With one,
      // a narrowly-scoped seed (e.g. the single-:wsId stream) leaves entities
      // outside its scope untouched instead of wiping every sibling.
      if (fresh.has(entity.id)) continue
      if (pruneScope && !pruneScope(entity)) continue
      await removeEntity(store, entity.id)
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
      // A rejection (a failed seed() GET, an IDB error) must NOT poison the chain:
      // .then on a rejected promise skips every subsequent step, which would
      // permanently freeze this stream for the session (no live frames, no
      // reconnect reseed could recover). Absorb it here so applyChain is always
      // resolved for the next step; applySeed throws before mutating the cache, so
      // a failed reseed simply leaves the cache intact and a later reseed retries.
      .catch((err: unknown) => {
        console.error(`entity-stream: seed failed for ${endpoint}`, err)
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
      // Absorb a failed frame apply so it can't poison the chain and freeze all
      // later frames + reseeds for the session (see runSeed). Ordering is still
      // preserved: a later frame only runs after this one's catch resolves.
      .catch((err: unknown) => {
        console.error(`entity-stream: frame apply failed for ${endpoint}`, err)
      })
  })

  return () => {
    disposed = true
    unsubscribe()
  }
}
