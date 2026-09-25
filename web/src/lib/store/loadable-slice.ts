import { idle, loading, success, failed, type Loadable } from '@/lib/loadable'
import { saveCache, loadCache, type CacheStoreName } from '@/lib/persistence/cache-store'
import { wsManager } from '@/lib/ws/manager'
import { isReconnectSentinel } from '@/lib/ws/types'

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
  /**
   * For a socket whose frames are complete entities (a snapshot on subscribe,
   * then one whole DTO per change): fold one frame into the held value, or
   * undefined when this frame cannot be merged (a reconnect sentinel) and the
   * value must be re-read. Return `current` itself for a frame that changes
   * nothing. Without it, every frame schedules a debounced re-read.
   */
  mergeFrame?: (current: T, frame: unknown) => T | undefined
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
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- the key tuple K is erased here; any K is accepted
type Setter<T> = (partial: Partial<LoadableSlice<T, any>>) => void
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- the key tuple K is erased here; any K is accepted
type Getter<T> = () => LoadableSlice<T, any>

export function createLoadableSlice<T, K extends unknown[] = [string]>(cfg: LoadableConfig<T, K>) {
  const keyOf = (...args: K): string => (cfg.cacheKey ? cfg.cacheKey(...args) : (args[0] as string))

  return (set: Setter<T>, get: Getter<T>): LoadableSlice<T, K> => {
    // A slice without `mergeFrame` answers each frame with a re-read; debouncing
    // collapses a snapshot burst (and rapid mutations) into a single request.
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
    /** The most recently issued read. A superseded read settles with it, so
     *  whoever awaited fetch() always reads the newest answer, never a store
     *  its own read left unpublished. */
    let latestRun: Promise<void> = Promise.resolve()
    /** Per key, the fetch whose request has not been SENT yet. A caller that
     *  arrives before the send is served by it — the answer is still at least
     *  as new as the moment it asked. A caller after the send issues its own,
     *  unless the key is live (below). */
    const unsent = new Map<string, Promise<void>>()
    /** Per key, the newest read in flight. */
    const running = new Map<string, Promise<void>>()
    /** Keys whose socket frames are merged into the held value (`startSync`
     *  with `mergeFrame`): frames replay onto a read in flight and keep a held
     *  answer current, so neither needs a second request. */
    const live = new Map<string, number>()
    /** The key the published value belongs to (the slice holds one value). */
    let heldKey: string | undefined
    /** Per key, one list per fetch in flight of the frames that arrived since
     *  it began: its answer may predate them, so they are replayed onto it. */
    const inFlightFrames = new Map<string, Set<unknown[]>>()

    const replay = (fresh: T, frames: readonly unknown[]): T =>
      frames.reduce<T>((value, frame) => cfg.mergeFrame?.(value, frame) ?? value, fresh)

    const run = async (key: string, args: K): Promise<void> => {
      const seq = ++latestFetch
      let sent = false
      const frames: unknown[] = []
      const tracked = inFlightFrames.get(key) ?? new Set<unknown[]>()
      tracked.add(frames)
      inFlightFrames.set(key, tracked)
      try {
        const cached = await loadCache<T>(cfg.store, key)
        if (seq !== latestFetch) return latestRun
        set({
          data: loading(cached ? success(cached.data, cached.fetchedAt) : get().data),
        })
        try {
          unsent.delete(key)
          sent = true
          const fresh = replay(await cfg.fetcher(...args), frames)
          if (seq !== latestFetch) return latestRun
          // An unchanged answer is not re-written: a warm boot otherwise
          // re-puts every cached list it just read.
          if (!cached || !sameJson(cached.data, fresh)) await saveCache(cfg.store, key, fresh)
          // Re-checked after the write too: a supersede can land inside it.
          if (seq !== latestFetch) return latestRun
          heldKey = key
          set({ data: success(fresh) })
        } catch (err) {
          if (seq !== latestFetch) return latestRun
          set({ data: failed(err as Error, get().data) })
        }
      } finally {
        if (!sent) unsent.delete(key)
        tracked.delete(frames)
      }
    }

    /** Issue a read unconditionally. `run` always yields (on the cache read)
     *  before it sends or checks for a supersede, so it is registered here
     *  before anything could need to join or settle with it. */
    const read = (key: string, args: K): Promise<void> => {
      const promise = run(key, args)
      latestRun = promise
      unsent.set(key, promise)
      running.set(key, promise)
      void promise.finally(() => {
        if (running.get(key) === promise) running.delete(key)
      })
      return promise
    }

    const scheduleRefetch = (key: string, args: K): void => {
      const pending = deltaTimers.get(key)
      if (pending) clearTimeout(pending)
      deltaTimers.set(
        key,
        setTimeout(() => {
          deltaTimers.delete(key)
          // Forced: a frame that cannot be merged (a reconnect) means the held
          // value may have missed changes, live or not.
          void read(key, args)
        }, DELTA_DEBOUNCE_MS),
      )
    }

    return {
      data: idle() as Loadable<T>,

      fetch: (...args: K) => {
        const key = keyOf(...args)
        const waiting = unsent.get(key)
        if (waiting) return waiting
        if (live.has(key)) {
          const inFlight = running.get(key)
          if (inFlight) return inFlight
          if (heldKey === key && get().data.status === 'success') return Promise.resolve()
        }
        return read(key, args)
      },

      startSync: (...args: K) => {
        if (!cfg.wsEndpoint) return () => {}
        const key = keyOf(...args)
        const merges = cfg.mergeFrame !== undefined
        if (merges) live.set(key, (live.get(key) ?? 0) + 1)
        const unsubscribe = wsManager.subscribe(cfg.wsEndpoint(...args), (event) => {
          void get().applyDelta(event, ...(args as unknown[] as K))
        })
        return () => {
          unsubscribe()
          if (!merges) return
          const holders = (live.get(key) ?? 1) - 1
          if (holders > 0) live.set(key, holders)
          else live.delete(key)
        }
      },

      applyDelta: async (event: unknown, ...args: K) => {
        const key = keyOf(...args)
        const merge = cfg.mergeFrame
        if (merge && !isReconnectSentinel(event)) {
          const inFlight = inFlightFrames.get(key) ?? new Set<unknown[]>()
          const current = get().data
          const next = current.status === 'success' ? merge(current.data, event) : undefined
          if (next !== undefined && current.status === 'success') {
            for (const frames of inFlight) frames.push(event)
            if (next === current.data) return
            heldKey = key
            set({ data: success(next, current.fetchedAt) })
            void saveCache(cfg.store, key, next)
            return
          }
          // Nothing held yet: the read in flight carries this frame.
          if (current.status !== 'success' && inFlight.size > 0) {
            for (const frames of inFlight) frames.push(event)
            return
          }
        }
        scheduleRefetch(key, args)
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
