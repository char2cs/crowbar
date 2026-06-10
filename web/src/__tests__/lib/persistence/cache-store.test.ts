import { describe, it, expect, beforeEach, vi } from 'vitest'
import * as idb from '@/lib/persistence/idb'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache, loadCache } from '@/lib/persistence/cache-store'

beforeEach(() => {
  resetDB()
  vi.restoreAllMocks()
})

describe('cache-store', () => {
  it('returns undefined when nothing cached', async () => {
    expect(await loadCache('projects-data', 'projects')).toBeUndefined()
  })

  it('saves and loads a record with fetchedAt', async () => {
    await saveCache('projects-data', 'projects', [{ id: 'a' }], 123)
    const rec = await loadCache<{ id: string }[]>('projects-data', 'projects')
    expect(rec?.data).toEqual([{ id: 'a' }])
    expect(rec?.fetchedAt).toBe(123)
  })

  it('overwrites an existing key', async () => {
    await saveCache('projects-data', 'projects', [{ id: 'a' }], 1)
    await saveCache('projects-data', 'projects', [{ id: 'b' }], 2)
    const rec = await loadCache<{ id: string }[]>('projects-data', 'projects')
    expect(rec?.data).toEqual([{ id: 'b' }])
    expect(rec?.fetchedAt).toBe(2)
  })

  // Resilience: a stale schema (missing object store) or any IDB failure must
  // degrade to a cache miss / no-op — never throw and break the caller's fetch.
  it('loadCache returns undefined when IDB throws', async () => {
    vi.spyOn(idb, 'getDB').mockRejectedValueOnce(
      new Error('NotFoundError: object store not found') as never,
    )
    expect(await loadCache('branch-review-data', 'ws3:diff')).toBeUndefined()
  })

  it('saveCache does not throw when IDB fails', async () => {
    vi.spyOn(idb, 'getDB').mockRejectedValueOnce(
      new Error('NotFoundError: object store not found') as never,
    )
    await expect(saveCache('branch-review-data', 'ws3:diff', { x: 1 })).resolves.toBeUndefined()
  })
})
