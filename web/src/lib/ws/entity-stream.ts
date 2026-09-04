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
/**
 * What a given `onChange` call is reporting. A consumer that only needs "the
 * cache moved" can ignore it; one that maintains a derived tree uses it to
 * merge a single entity by id instead of rebuilding everything — a seed is a
 * whole-scope replacement (and can prune), a frame is exactly one entity.
 */
export type EntityChange =
  /** A full GET seed (startup or reconnect reseed) has committed. */
  | { kind: 'seed' }
  /** One live DTO frame has committed; `frame.status === 'deleted'` is a tombstone. */
  | { kind: 'frame'; frame: EntityFrame }

export interface SubscribeEntityStreamOptions<T> {
  /** Hierarchical WS endpoint, e.g. '/v0/projects/:p/repos/:r/workspaces'. */
  endpoint: string
  /** The entity object store the DTOs are keyed into. */
  store: EntityStoreName
  /** One-time GET that returns the full current set for this scope. */
  seed: () => Promise<T[]>
  /** Called after every cache mutation (seed batch, upsert, remove), with what
   *  changed so the caller can merge incrementally rather than rebuild. */
  onChange?: (change: EntityChange) => void
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
  /**
   * Turn ONE raw frame into the entity it is about, or null to ignore it.
   *
   * Every frame goes through this BEFORE the `id` check, so a feed whose frames
   * are not entity DTOs at all can still drive this cache. That is now the
   * normal case for `crowbar_workspaces`: a worktree is held by a chat, so its
   * live updates ride the chat LIFECYCLE socket, where a `worktree_state` event
   * carries the worktree nested inside it and every other kind
   * (`turn_started`, `deleted`, `folder_created`, …) is about something else
   * entirely and maps to null.
   *
   * Omitted, the frame is cast exactly as it always was.
   */
  mapFrame?: (raw: unknown) => T | null
}

export function subscribeEntityStream<T extends { id: string; status?: string }>(
  opts: SubscribeEntityStreamOptions<T>,
): () => void {
  const { endpoint, store, seed, onChange, pruneScope, mapFrame } = opts
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
    // Prune a ghost ONLY if this seed is authoritative over it. Without a
    // pruneScope the seed owns the whole store (legacy behaviour). With one, a
    // narrowly-scoped seed (e.g. the single-:wsId stream) leaves entities
    // outside its scope untouched instead of wiping every sibling. Any cached
    // entity present in `fresh` is skipped here, so this set and the upsert
    // set below are always disjoint (different ids) — each id gets its own
    // IDB transaction, so running them concurrently can't race on the same
    // row. The seed as a whole is still serialized against live frames via
    // applyChain above, so this doesn't reintroduce the upsert/delete-ordering
    // race (H21) that motivated that chain.
    const idsToPrune: string[] = []
    for (const entity of cached) {
      if (!fresh.has(entity.id) && (!pruneScope || pruneScope(entity))) {
        idsToPrune.push(entity.id)
      }
    }
    await Promise.all(idsToPrune.map((id) => removeEntity(store, id)))
    await Promise.all(items.map((item) => upsertEntity(store, item)))
  }

  function runSeed(): void {
    const generation = ++seedGeneration
    applyChain = applyChain
      .then(() => applySeed(generation))
      .then(() => {
        if (!disposed) onChange?.({ kind: 'seed' })
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
    // Mapped BEFORE the id check: on a lifecycle feed the raw frame has no `id`
    // of its own at all, and the entity it is about is nested inside it.
    // Tombstones are read off the MAPPED value below, so a mapper is free to
    // produce one.
    const mapped = mapFrame ? mapFrame(data) : (data as EntityFrame)
    if (mapped === null) return
    const frame = mapped as EntityFrame
    if (!frame || typeof frame.id !== 'string') return
    applyChain = applyChain
      .then(() => applyFrame(frame))
      .then(() => {
        if (!disposed) onChange?.({ kind: 'frame', frame })
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
