import { describe, it, expect, beforeEach } from 'vitest'
import { openDB, type IDBPDatabase } from 'idb'
import { IDBFactory } from 'fake-indexeddb'
import { getDB, resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
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

describe('idb schema v10', () => {
  const RETIRED = ['ui-preferences', 'workspace-hierarchy']

  it('never leaves the retired stores in a fresh database', async () => {
    const db = await getDB()
    for (const name of RETIRED) expect(Array.from(db.objectStoreNames)).not.toContain(name)
  })

  it('drops them from an existing v9 database and keeps everything else', async () => {
    const v9 = await openDB('crowbar', 9, {
      upgrade(db) {
        for (const name of RETIRED) db.createObjectStore(name)
        db.createObjectStore('sidebar-ui')
      },
    })
    await v9.put('ui-preferences', { theme: 'dark' }, 'global')
    await v9.put('sidebar-ui', { collapsedChatRows: ['c1'] }, 'global')
    v9.close()

    const db = await getDB()
    expect(db.version).toBeGreaterThanOrEqual(10)
    for (const name of RETIRED) expect(Array.from(db.objectStoreNames)).not.toContain(name)
    expect(await db.get('sidebar-ui', 'global')).toMatchObject({ collapsedChatRows: ['c1'] })
  })
  // The v9 database exactly as the base build (70ec430) left it, one record in
  // every store: the v10 upgrade may drop the two retired stores and nothing else.
  it('upgrades a base-build v9 database with data in every store, losing only the retired', async () => {
    const byKey = (key: string) => ({ keyPath: key })
    const stores: Array<[string, IDBObjectStoreParameters | undefined, unknown, IDBValidKey?]> = [
      ['workspace-layout', byKey('workspaceId'), { workspaceId: 'window', panes: {} }],
      ['editor-state', byKey('bufferId'), null],
      ['ui-preferences', undefined, { theme: 'dark' }, 'global'],
      ['sidebar-ui', undefined, { collapsedChatRows: ['c1'] }, 'global'],
      ['workspace-hierarchy', byKey('repoId'), { repoId: 'r1', entries: [] }],
      ['branch-review', byKey('wsId'), { wsId: 'ws1', viewed: {} }],
      ['workspaces-data', byKey('key'), { key: 'k', data: 1, fetchedAt: 1 }],
      ['git-data', byKey('key'), { key: 'k', data: 2, fetchedAt: 1 }],
      ['file-tree-data', byKey('key'), { key: 'k', data: 3, fetchedAt: 1 }],
      ['branch-review-data', byKey('key'), { key: 'k', data: 4, fetchedAt: 1 }],
      ['chat-history', byKey('key'), { key: 'k', data: 5, fetchedAt: 1 }],
      ['projects-data', byKey('key'), { key: 'k', data: 6, fetchedAt: 1 }],
      ['chats-data', byKey('key'), { key: 'k', data: 7, fetchedAt: 1 }],
      ['crowbar_projects', byKey('id'), { id: 'p1' }],
      ['crowbar_repos', byKey('id'), { id: 'r1' }],
      ['crowbar_workspaces', byKey('id'), { id: 'w1' }],
      ['crowbar_threads', byKey('id'), { id: 't1' }],
      ['crowbar_folders', byKey('id'), { id: 'f1' }],
      ['crowbar_chats', byKey('id'), { id: 'c1', workspaceId: 'w1' }],
    ]
    const editorState = { workspaceId: 'w1', bufferId: 'b1', cursorLine: 3 }
    const v9 = await openDB('crowbar', 9, {
      upgrade(db) {
        for (const [name, params] of stores) {
          if (name === 'editor-state') {
            db.createObjectStore(name, { keyPath: ['workspaceId', 'bufferId'] }).createIndex(
              'workspaceId',
              'workspaceId',
            )
          } else db.createObjectStore(name, params)
        }
      },
    })
    for (const [name, , value, key] of stores) {
      await v9.put(name, name === 'editor-state' ? editorState : value, key)
    }
    v9.close()

    const db = await getDB()
    const raw = db as unknown as IDBPDatabase
    const names = Array.from(db.objectStoreNames)
    for (const [name, params, value, key] of stores) {
      if (RETIRED.includes(name)) {
        expect(names).not.toContain(name)
        continue
      }
      const id = key ?? (name === 'editor-state' ? ['w1', 'b1'] : keyOf(value, params))
      expect(await raw.get(name, id), name).toEqual(name === 'editor-state' ? editorState : value)
    }
    expect(await db.getAllFromIndex('editor-state', 'workspaceId', 'w1')).toEqual([editorState])
  })
})

function keyOf(value: unknown, params: IDBObjectStoreParameters | undefined): IDBValidKey {
  return (value as Record<string, IDBValidKey>)[params!.keyPath as string]
}

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
