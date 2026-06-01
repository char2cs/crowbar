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

// Shim types to satisfy Zustand's StateCreator while keeping generic K flexible
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Setter<T> = (partial: Partial<LoadableSlice<T, any>>) => void
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Getter<T> = () => LoadableSlice<T, any>

export function createLoadableSlice<T, K extends unknown[] = [string]>(
  cfg: LoadableConfig<T, K>,
) {
  const keyOf = (...args: K): string => {
    if (cfg.cacheKey) return cfg.cacheKey(...args)
    const k = args[0]
    if (typeof k !== 'string') {
      throw new Error(
        `[loadable-slice] store "${cfg.store}": first arg is ${typeof k}; provide a cacheKey for non-string keys`,
      )
    }
    return k
  }

  return (set: Setter<T>, get: Getter<T>): LoadableSlice<T, K> => ({
    data: idle() as Loadable<T>,

    fetch: async (...args: K) => {
      const cached = await loadCache<T>(cfg.store, keyOf(...args))
      set({
        data: loading(cached ? success(cached.data, cached.fetchedAt) : get().data),
      })
      try {
        const fresh = await cfg.fetcher(...args)
        await saveCache(cfg.store, keyOf(...args), fresh)
        set({ data: success(fresh) })
      } catch (err) {
        set({ data: failed(err as Error, get().data) })
      }
    },

    startSync: (...args: K) => {
      if (!cfg.wsEndpoint) return () => {}
      return wsManager.subscribe(cfg.wsEndpoint(...args), (event) => {
        void get().applyDelta(event, ...args)
      })
    },

    applyDelta: async (_event: unknown, ...args: K) => {
      await get().fetch(...args)
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
  })
}
