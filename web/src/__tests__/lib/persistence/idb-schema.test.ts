import { describe, it, expect, beforeEach } from 'vitest'
import { getDB, resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  resetDB()
})

describe('idb schema v5', () => {
  it('creates the six new local-first stores', async () => {
    const db = await getDB()
    const names = Array.from(db.objectStoreNames)
    expect(names).toContain('workspaces-data')
    expect(names).toContain('git-data')
    expect(names).toContain('file-tree-data')
    expect(names).toContain('branch-review-data')
    expect(names).toContain('chat-history')
    expect(names).toContain('projects-data')
  })

  it('drops the query-cache store', async () => {
    const db = await getDB()
    expect(Array.from(db.objectStoreNames)).not.toContain('query-cache')
  })

  it('round-trips a record through git-data', async () => {
    const db = await getDB()
    await db.put('git-data', { key: '/repo', data: { n: 1 }, fetchedAt: 42 })
    const rec = await db.get('git-data', '/repo')
    expect(rec?.fetchedAt).toBe(42)
  })
})
