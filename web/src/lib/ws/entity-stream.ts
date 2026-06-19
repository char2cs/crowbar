import { wsManager } from './manager'
import { upsertEntity, removeEntity, type EntityStoreName } from '@/lib/persistence/entity-cache'
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

  async function runSeed(): Promise<void> {
    const items = await seed()
    if (disposed) return
    for (const item of items) {
      await upsertEntity(store, item)
    }
    if (!disposed) onChange?.()
  }

  // Seed before the first push lands; the WS subscription is registered
  // synchronously below so no frame is dropped while the GET is in flight.
  void runSeed()

  const unsubscribe = wsManager.subscribe(endpoint, (data: unknown) => {
    if (disposed) return
    // The reconnect sentinel is NOT a DTO — never upsert it; trigger a full
    // GET reseed so any frames missed during the outage are recovered.
    if (isReconnectSentinel(data)) {
      void runSeed()
      return
    }
    const frame = data as EntityFrame
    if (!frame || typeof frame.id !== 'string') return
    void (async () => {
      if (frame.status === 'deleted') {
        await removeEntity(store, frame.id)
      } else {
        await upsertEntity(store, frame as unknown as T)
      }
      if (!disposed) onChange?.()
    })()
  })

  return () => {
    disposed = true
    unsubscribe()
  }
}
