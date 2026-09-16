import { describe, it, expect, beforeEach, vi } from 'vitest'
import { create } from 'zustand'

// The cache WRITE `fetch` awaits after its last supersede check, wrapped so one
// test can be inside that exact window when a newer fetch is issued. Defaults
// straight through to the real one, so every other test here is unaffected.
const { saveCacheSpy, realSaveCache } = vi.hoisted(() => ({
  saveCacheSpy: vi.fn(),
  realSaveCache: { fn: null as null | ((...a: unknown[]) => Promise<void>) },
}))

vi.mock('@/lib/persistence/cache-store', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/persistence/cache-store')>()
  realSaveCache.fn = actual.saveCache as (...a: unknown[]) => Promise<void>
  return { ...actual, saveCache: (...a: unknown[]) => saveCacheSpy(...a) }
})

import { resetDB } from '@/lib/persistence/idb'
import { saveCache } from '@/lib/persistence/cache-store'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { dataOf } from '@/lib/loadable'

beforeEach(() => {
  resetDB()
  saveCacheSpy.mockReset()
  saveCacheSpy.mockImplementation((...a: unknown[]) => realSaveCache.fn!(...a))
})

/** A promise the test resolves by hand, to hold an async step open at an exact
 *  point. (`Promise.withResolvers` is not in this project's TS lib target.) */
function deferred<T = void>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

function makeStore(fetcher: (key: string) => Promise<number[]>) {
  return create<LoadableSlice<number[]>>()((set, get) =>
    createLoadableSlice<number[]>({
      store: 'projects-data',
      fetcher,
    })(set, get),
  )
}

// A fetcher whose every call parks until explicitly released, exposing both the
// release handles and a signal for "call N has started". fetch() awaits an IDB
// read BEFORE invoking the fetcher, so a test must block on the START signal
// rather than assume the fetcher ran synchronously — and blocking on a real
// signal is what keeps this deterministic instead of timing-dependent.
function parkedFetcher(resultFor: (key: string) => number[] | Error) {
  const release: Array<() => void> = []
  const started: Array<() => void> = []
  const startedAt = [0, 1].map(
    (i) =>
      new Promise<void>((resolve) => {
        started[i] = resolve
      }),
  )
  const fetcher = (key: string): Promise<number[]> =>
    new Promise<number[]>((resolve, reject) => {
      const outcome = resultFor(key)
      release.push(() => (outcome instanceof Error ? reject(outcome) : resolve(outcome)))
      started.shift()?.()
    })
  return { fetcher, release, startedAt }
}

describe('createLoadableSlice', () => {
  it('fetch writes IDB and sets success', async () => {
    const store = makeStore(async () => [1, 2, 3])
    await store.getState().fetch('projects')
    expect(store.getState().data.status).toBe('success')
    expect(dataOf(store.getState().data)).toEqual([1, 2, 3])
    const { loadCache } = await import('@/lib/persistence/cache-store')
    expect((await loadCache('projects-data', 'projects'))?.data).toEqual([1, 2, 3])
  })

  it('fetch failure preserves stale data from IDB', async () => {
    await saveCache('projects-data', 'projects', [9, 9], 100)
    const store = makeStore(async () => {
      throw new Error('boom')
    })
    await store.getState().fetch('projects')
    expect(store.getState().data.status).toBe('error')
    expect(dataOf(store.getState().data)).toEqual([9, 9])
  })

  // The sidebar rebuild (app-sync-provider's rebuildSidebar) calls fetch() on EVERY
  // entity-stream frame, undebounced, from five streams sharing one callback — so
  // fetches overlap routinely. Without a sequence guard the LAST FETCH TO RESOLVE
  // wins, which is not the last one ISSUED: an older, slower fetch overwrites the
  // newer result with the state it read before the newer frame landed. That is how
  // a workspace's `working` overlay wedges ON — the daemon sends `working:false`
  // exactly once and never repeats it, so a stale row is never repaired.
  it('a slower earlier fetch does not overwrite a newer fetch result', async () => {
    const { fetcher, release, startedAt } = parkedFetcher((key) => (key === 'first' ? [1] : [2]))
    const store = makeStore(fetcher)

    const first = store.getState().fetch('first')
    await startedAt[0]
    const second = store.getState().fetch('second')
    await startedAt[1]

    // Resolve the NEWER fetch first, then let the older one land late.
    release[1]()
    release[0]()
    await Promise.all([first, second])

    expect(dataOf(store.getState().data)).toEqual([2])
  })

  it('a superseded fetch does not persist its stale result to the cache', async () => {
    const { fetcher, release, startedAt } = parkedFetcher((key) => (key === 'stale' ? [1] : [2]))
    const store = makeStore(fetcher)

    const stale = store.getState().fetch('stale')
    await startedAt[0]
    const fresh = store.getState().fetch('fresh')
    await startedAt[1]

    release[1]()
    release[0]()
    await Promise.all([stale, fresh])

    const { loadCache } = await import('@/lib/persistence/cache-store')
    expect(await loadCache('projects-data', 'stale')).toBeUndefined()
  })

  // The same race, one await later: the guard above is checked BEFORE the cache
  // write, and the store write happens AFTER it. A fetch superseded while that
  // write is in flight used to publish its stale result anyway — and publish it
  // LAST, on top of the winner. A caller that tells a fresh publication from a
  // stale one by object IDENTITY (app-sync-provider's seed gate does, because
  // `success(old)` and `success(new)` are the same shape) then reads a pre-write
  // snapshot as a settled, brand-new one.
  it('a fetch superseded while its cache write is in flight does not land its stale result', async () => {
    const { fetcher, release, startedAt } = parkedFetcher((key) => (key === 'stale' ? [1] : [2]))
    const store = makeStore(fetcher)

    const parked = deferred()
    const reachedWrite = deferred()
    saveCacheSpy.mockImplementationOnce(async (...args: unknown[]) => {
      reachedWrite.resolve()
      await parked.promise
      return realSaveCache.fn!(...args)
    })

    const stale = store.getState().fetch('stale')
    await startedAt[0]
    release[0]()
    // A real signal, not a delay: the older fetch is now PAST its last
    // supersede check, inside the write.
    await reachedWrite.promise

    const fresh = store.getState().fetch('fresh')
    await startedAt[1]
    release[1]()
    await fresh
    expect(dataOf(store.getState().data)).toEqual([2])

    parked.resolve()
    await stale

    expect(dataOf(store.getState().data)).toEqual([2])
  })

  it('optimisticWrite commits on success', async () => {
    const store = makeStore(async () => [1])
    await store.getState().optimisticWrite([7], async () => [7])
    expect(dataOf(store.getState().data)).toEqual([7])
  })

  it('optimisticWrite rolls back and rethrows on failure', async () => {
    const store = makeStore(async () => [1])
    await store.getState().fetch('projects')
    await expect(
      store.getState().optimisticWrite([7], async () => {
        throw new Error('no')
      }),
    ).rejects.toThrow('no')
    expect(dataOf(store.getState().data)).toEqual([1])
  })
})
