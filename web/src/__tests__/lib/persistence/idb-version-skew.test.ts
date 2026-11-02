/**
 * Opening the database when what is on disk does not match what this build
 * declares.
 *
 * Both cases below were found by running the app, not by reading the code, and
 * both fail the same way: silently. Every entity-cache call is best-effort, so
 * a database that cannot be opened or is missing a store does not raise — the
 * feature behind it simply has no data, forever, with a green typecheck and a
 * green suite.
 *
 * The skew is ordinary rather than exotic: every build carrying the same bundle
 * id shares one origin, so a checkout, a second checkout and an installed
 * release all open the same database, and running an older one after a newer
 * one is routine.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { openDB } from 'idb'
import { IDBFactory } from 'fake-indexeddb'
import { getDB, resetDB } from '@/lib/persistence/idb'

// The stores the app expects to exist. Kept as a literal rather than imported:
// this file is asserting the contract, so reading it from the module under test
// would let both sides drift together.
const ENTITY_STORES = [
  'crowbar_projects',
  'crowbar_repos',
  'crowbar_workspaces',
  'crowbar_threads',
  'crowbar_folders',
  'crowbar_chats',
] as const

// A fresh factory rather than `deleteDB`: the module keeps its connection open
// behind a singleton, and a delete blocks on any open connection — which is a
// hang, not a failure, so the suite would sit at its timeout instead of telling
// you anything.
beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  resetDB()
})

afterEach(() => {
  resetDB()
})

describe('a database newer than this build', () => {
  it('opens it as it is instead of throwing VersionError', async () => {
    // A newer build has been here first. Asking for a LOWER version is not a
    // no-op — it rejects, and the rejection costs the whole local cache.
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

describe('concurrent callers', () => {
  it('share ONE open, so a version change cannot deadlock against itself', async () => {
    // Memoizing only the settled handle lets every caller that arrives during
    // startup begin its own open. That is harmless until an open has to change
    // the version — then the second connection blocks the upgrade, the upgrade
    // holds the second open, and every cache read hangs for the session.
    const older = await openDB('crowbar', 9, {
      upgrade(db) {
        for (const name of ENTITY_STORES) db.createObjectStore(name, { keyPath: 'id' })
      },
    })
    older.close()

    const [a, b, c] = await Promise.all([getDB(), getDB(), getDB()])
    expect(a).toBe(b)
    expect(b).toBe(c)
    expect(a.version).toBe(11)
  })
})
