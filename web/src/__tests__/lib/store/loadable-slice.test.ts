import { describe, it, expect, beforeEach } from 'vitest'
import { create } from 'zustand'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache } from '@/lib/persistence/cache-store'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { dataOf } from '@/lib/loadable'

beforeEach(() => { resetDB() })

function makeStore(fetcher: (key: string) => Promise<number[]>) {
  return create<LoadableSlice<number[]>>()((set, get) =>
    createLoadableSlice<number[]>({
      store: 'projects-data',
      fetcher,
    })(set, get),
  )
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
    const store = makeStore(async () => { throw new Error('boom') })
    await store.getState().fetch('projects')
    expect(store.getState().data.status).toBe('error')
    expect(dataOf(store.getState().data)).toEqual([9, 9])
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
      store.getState().optimisticWrite([7], async () => { throw new Error('no') }),
    ).rejects.toThrow('no')
    expect(dataOf(store.getState().data)).toEqual([1])
  })
})
