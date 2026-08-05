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

describe('idb schema v7 entity stores', () => {
  it('creates the four entity stores keyed by id', async () => {
    const db = await getDB()
    const names = Array.from(db.objectStoreNames)
    expect(names).toContain('crowbar_projects')
    expect(names).toContain('crowbar_repos')
    expect(names).toContain('crowbar_workspaces')
    expect(names).toContain('crowbar_threads')
  })

  it('creates the v8 folder store keyed by id', async () => {
    // A new object store only comes into existence inside an upgrade callback,
    // so shipping one without bumping the version leaves an existing install
    // with no store at all — and every entity-cache write swallows that, so it
    // fails silently at runtime instead of loudly at build time.
    const db = await getDB()
    expect(Array.from(db.objectStoreNames)).toContain('crowbar_folders')
    await db.put('crowbar_folders', { id: 'f1', name: 'spikes' } as never)
    const rec = await db.get('crowbar_folders', 'f1')
    expect((rec as { name: string } | undefined)?.name).toBe('spikes')
  })

  it('round-trips an entity keyed by id through crowbar_workspaces', async () => {
    const db = await getDB()
    await db.put('crowbar_workspaces', { id: 'w1', branch: 'main' } as never)
    const rec = await db.get('crowbar_workspaces', 'w1')
    expect((rec as { branch: string } | undefined)?.branch).toBe('main')
  })

  it('keeps the existing local-first stores (additive upgrade)', async () => {
    const db = await getDB()
    const names = Array.from(db.objectStoreNames)
    expect(names).toContain('projects-data')
    expect(names).toContain('chats-data')
  })
})
