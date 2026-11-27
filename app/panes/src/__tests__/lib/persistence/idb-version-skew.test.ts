import { describe, it, expect, beforeEach } from 'vitest'
import { openDB } from 'idb'
import { IDBFactory } from 'fake-indexeddb'
import { getDB } from '@/lib/persistence/idb'
import { hydrateCriticalStores } from '@/lib/boot'

const ENTITY_STORES = [
  'crowbar_projects',
  'crowbar_repos',
  'crowbar_workspaces',
  'crowbar_folders',
  'crowbar_chats',
] as const

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
})

describe('a database newer than this build', () => {
  it('opens it as it is instead of throwing VersionError', async () => {
    const ahead = await openDB('crowbar', 99, {
      upgrade(db) {
        for (const name of ENTITY_STORES) db.createObjectStore(name, { keyPath: 'id' })
      },
    })
    ahead.close()

    const db = await getDB()
    expect(db.version).toBe(99)
    for (const name of ENTITY_STORES) {
      expect(Array.from(db.objectStoreNames), `lost ${name}`).toContain(name)
    }
  })

  it('still reads and writes through the cache it inherited', async () => {
    const ahead = await openDB('crowbar', 99, {
      upgrade(db) {
        for (const name of ENTITY_STORES) db.createObjectStore(name, { keyPath: 'id' })
      },
    })
    ahead.close()

    const db = await getDB()
    await db.put('crowbar_folders', {
      id: 'f1',
      repoId: 'r1',
      projectId: 'p1',
      name: 'spikes',
      order: 0,
    })
    expect(await db.get('crowbar_folders', 'f1')).toMatchObject({ name: 'spikes' })
  })
})

describe('a database already at this build’s version', () => {
  it('boots on it without changing its version or its stores', async () => {
    const earlier = await getDB()
    await earlier.put('crowbar_chats', { id: 'c1', title: 'kept' } as never)
    const stores = Array.from(earlier.objectStoreNames).sort()
    localStorage.setItem('crowbar:cache-version', '2')

    await hydrateCriticalStores()

    const after = await openDB('crowbar')
    expect(after.version).toBe(16)
    expect(Array.from(after.objectStoreNames).sort()).toEqual(stores)
    expect(await after.get('crowbar_chats', 'c1')).toEqual({ id: 'c1', title: 'kept' })
    after.close()
    expect(localStorage.getItem('crowbar:cache-version')).toBe('2')
  })
})

describe('concurrent callers', () => {
  it('share ONE open, so a version change cannot deadlock against itself', async () => {
    const older = await openDB('crowbar', 9, {
      upgrade(db) {
        for (const name of ENTITY_STORES) db.createObjectStore(name, { keyPath: 'id' })
      },
    })
    older.close()

    const [a, b, c] = await Promise.all([getDB(), getDB(), getDB()])
    expect(a).toBe(b)
    expect(b).toBe(c)
    expect(a.version).toBe(16)
  })
})

describe('another window upgrading', () => {
  it('is not held by this connection, and the next call opens at the new version', async () => {
    const held = await getDB()
    const upgraded = await openDB('crowbar', 99)
    upgraded.close()

    const reopened = await getDB()
    expect(reopened).not.toBe(held)
    expect(reopened.version).toBe(99)
  })
})
