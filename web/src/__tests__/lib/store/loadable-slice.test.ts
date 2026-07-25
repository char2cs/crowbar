import { describe, it, expect, beforeEach } from 'vitest'
import { create } from 'zustand'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache } from '@/lib/persistence/cache-store'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { dataOf } from '@/lib/loadable'

beforeEach(() => {
  resetDB()
})

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
