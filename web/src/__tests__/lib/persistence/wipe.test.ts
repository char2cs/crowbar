import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { upsertEntity, getAllEntities } from '@/lib/persistence/entity-cache'
import {
  wipeEntityCache,
  maybeWipeOnVersionChange,
  CROWBAR_CACHE_VERSION,
} from '@/lib/persistence/idb'

const VERSION_KEY = 'crowbar:cache-version'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  localStorage.clear()
})

async function seedAllStores(): Promise<void> {
  await upsertEntity('crowbar_projects', { id: 'p' })
  await upsertEntity('crowbar_repos', { id: 'r' })
  await upsertEntity('crowbar_workspaces', { id: 'w' })
  await upsertEntity('crowbar_threads', { id: 't' })
}

describe('wipeEntityCache', () => {
  it('clears all four crowbar_* entity stores', async () => {
    await seedAllStores()
    await wipeEntityCache()
    expect(await getAllEntities('crowbar_projects')).toHaveLength(0)
    expect(await getAllEntities('crowbar_repos')).toHaveLength(0)
    expect(await getAllEntities('crowbar_workspaces')).toHaveLength(0)
    expect(await getAllEntities('crowbar_threads')).toHaveLength(0)
  })
})

describe('maybeWipeOnVersionChange', () => {
  it('wipes the entity stores on a version mismatch and bumps the stored version', async () => {
    localStorage.setItem(VERSION_KEY, 'stale-version')
    await seedAllStores()

    await maybeWipeOnVersionChange()

    expect(await getAllEntities('crowbar_projects')).toHaveLength(0)
    expect(await getAllEntities('crowbar_workspaces')).toHaveLength(0)
    expect(localStorage.getItem(VERSION_KEY)).toBe(CROWBAR_CACHE_VERSION)
  })

  it('is a no-op when the stored version already matches', async () => {
    localStorage.setItem(VERSION_KEY, CROWBAR_CACHE_VERSION)
    await seedAllStores()

    await maybeWipeOnVersionChange()

    expect(await getAllEntities('crowbar_projects')).toHaveLength(1)
    expect(await getAllEntities('crowbar_repos')).toHaveLength(1)
    expect(await getAllEntities('crowbar_workspaces')).toHaveLength(1)
    expect(await getAllEntities('crowbar_threads')).toHaveLength(1)
  })

  it('wipes and records the version on first run (no stored version)', async () => {
    await seedAllStores()
    expect(localStorage.getItem(VERSION_KEY)).toBeNull()

    await maybeWipeOnVersionChange()

    expect(await getAllEntities('crowbar_projects')).toHaveLength(0)
    expect(localStorage.getItem(VERSION_KEY)).toBe(CROWBAR_CACHE_VERSION)
  })
})
