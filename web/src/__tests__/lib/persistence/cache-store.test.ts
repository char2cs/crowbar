import { describe, it, expect, beforeEach } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache, loadCache } from '@/lib/persistence/cache-store'

beforeEach(() => { resetDB() })

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
})
