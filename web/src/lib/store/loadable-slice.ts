import { idle, loading, success, failed, type Loadable } from '@/lib/loadable'
import { saveCache, loadCache, type CacheStoreName } from '@/lib/persistence/cache-store'
import { wsManager } from '@/lib/ws/manager'

export interface LoadableSlice<T, K extends unknown[] = [string]> {
  data: Loadable<T>
  fetch: (...args: K) => Promise<void>
  startSync: (...args: K) => () => void
  applyDelta: (event: unknown, ...args: K) => Promise<void>
  optimisticWrite: (optimistic: T, commit: () => Promise<T | void>) => Promise<void>
}

interface LoadableConfig<T, K extends unknown[]> {
  store: CacheStoreName
  fetcher: (...args: K) => Promise<T>
  cacheKey?: (...args: K) => string
  wsEndpoint?: (...args: K) => string
}

const DELTA_DEBOUNCE_MS = 120

function sameJson(a: unknown, b: unknown): boolean {
  try {
    return JSON.stringify(a) === JSON.stringify(b)
  } catch {
    return false
  }
}

// Shim types to satisfy Zustand's StateCreator while keeping generic K flexible
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Setter<T> = (partial: Partial<LoadableSlice<T, any>>) => void
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Getter<T> = () => LoadableSlice<T, any>

export function createLoadableSlice<T, K extends unknown[] = [string]>(cfg: LoadableConfig<T, K>) {
  const keyOf = (...args: K): string => (cfg.cacheKey ? cfg.cacheKey(...args) : (args[0] as string))

  return (set: Setter<T>, get: Getter<T>): LoadableSlice<T, K> => {
    // Snapshot-on-subscribe delivers one WS event per entity; debouncing the
    // refetch collapses that burst (and rapid mutations) into a single request.
    const deltaTimers = new Map<string, ReturnType<typeof setTimeout>>()

    // Only the most-recently ISSUED fetch may write. Callers overlap routinely —
    // the sidebar's rebuildSidebar() runs one fetch per entity-stream frame,
    // undebounced, from five streams sharing the callback — and resolution order
    // is not issue order, so without this the LAST FETCH TO RESOLVE wins. An older,
    // slower fetch then overwrites a newer result with the state it read before the
    // newer frame landed, and persists that stale snapshot to the cache on the way
    // out. Nothing repairs it: the daemon sends a delta like a workspace's
    // `working:false` exactly once, so a row that loses this race stays wrong until
    // the next unrelated frame — which is how an idle workspace kept spinning.
    let latestFetch = 0
    /** Per key, the fetch whose request has not been SENT yet. A caller that
     *  arrives before the send is served by it — the answer is still at least
     *  as new as the moment it asked — so a boot burst (route guard, background
     *  hydrate, sync engine) costs one request, not one each. A caller after
     *  the send issues its own, so no one is ever handed an answer older than
     *  its call. */
    const unsent = new Map<string, Promise<void>>()

    const run = async (key: string, args: K): Promise<void> => {
      const seq = ++latestFetch
      let sent = false
      try {
        const cached = await loadCache<T>(cfg.store, key)
        if (seq !== latestFetch) return
        set({
          data: loading(cached ? success(cached.data, cached.fetchedAt) : get().data),
        })
        try {
          unsent.delete(key)
          sent = true
          const fresh = await cfg.fetcher(...args)
          if (seq !== latestFetch) return
          // An unchanged answer is not re-written: a warm boot otherwise
          // re-puts every cached list it just read.
          if (!cached || !sameJson(cached.data, fresh)) await saveCache(cfg.store, key, fresh)
          // Re-checked AFTER the write, not only before it: the cache write is
          // an await like any other, and a supersede that lands inside it would
          // otherwise still publish here — last, on top of the winner. Same
          // stale-write bug the guards above prevent, one await later.
          if (seq !== latestFetch) return
          set({ data: success(fresh) })
        } catch (err) {
          if (seq !== latestFetch) return
          set({ data: failed(err as Error, get().data) })
        }
      } finally {
        if (!sent) unsent.delete(key)
      }
    }

    return {
      data: idle() as Loadable<T>,

      fetch: (...args: K) => {
        const key = keyOf(...args)
        const waiting = unsent.get(key)
        if (waiting) return waiting
        // `run` always yields (on the cache read) before it sends, so it is
        // registered here before anything could need to join it.
        const promise = run(key, args)
        unsent.set(key, promise)
        return promise
      },

      startSync: (...args: K) => {
        if (!cfg.wsEndpoint) return () => {}
        return wsManager.subscribe(cfg.wsEndpoint(...args), (event) => {
          void get().applyDelta(event, ...(args as unknown[] as K))
        })
      },

      applyDelta: async (_event: unknown, ...args: K) => {
        const key = keyOf(...args)
        const pending = deltaTimers.get(key)
        if (pending) clearTimeout(pending)
        deltaTimers.set(
          key,
          setTimeout(() => {
            deltaTimers.delete(key)
            void get().fetch(...args)
          }, DELTA_DEBOUNCE_MS),
        )
      },

      optimisticWrite: async (optimistic: T, commit: () => Promise<T | void>) => {
        const prev = get().data
        set({ data: success(optimistic) })
        try {
          const confirmed = await commit()
          if (confirmed !== undefined) set({ data: success(confirmed) })
        } catch (err) {
          set({ data: prev })
          throw err
        }
      },
    }
  }
}
