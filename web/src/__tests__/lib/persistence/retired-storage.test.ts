import { beforeEach, describe, expect, it } from 'vitest'
import { openDB } from 'idb'
import { IDBFactory } from 'fake-indexeddb'
import { retireOrphanedStorage } from '@/lib/persistence/retired-storage'

// Exactly what the base build (70ec430) wrote and nothing in this build reads.
const RETIRED_SETTINGS = {
  'crowbar:settings:editorEngine': '"monaco"',
  'crowbar:settings:formatter': '"prettier"',
  'crowbar:settings:lintOnSave': 'false',
  'crowbar:settings:externalEditor': '"none"',
  'crowbar:settings:customEditorCommand': '""',
  'debug-scroll': 'true',
}

describe('retireOrphanedStorage', () => {
  beforeEach(() => {
    localStorage.clear()
    globalThis.indexedDB = new IDBFactory()
  })

  it('removes the retired localStorage keys and leaves live ones alone', async () => {
    for (const [key, value] of Object.entries(RETIRED_SETTINGS)) localStorage.setItem(key, value)
    localStorage.setItem('crowbar:settings:fontFamily', '"Geist Mono Variable"')

    await retireOrphanedStorage()

    for (const key of Object.keys(RETIRED_SETTINGS)) expect(localStorage.getItem(key)).toBeNull()
    expect(localStorage.getItem('crowbar:settings:fontFamily')).toBe('"Geist Mono Variable"')
  })

  it('deletes the retired tree-sitter parser cache database, and only it', async () => {
    const parsers = await openDB('crowbar-parser-cache', 1, {
      upgrade(db) {
        db.createObjectStore('parsers', { keyPath: 'languageId' })
      },
    })
    await parsers.put('parsers', { languageId: 'typescript', size: 1 })
    parsers.close()
    ;(await openDB('crowbar', 1, { upgrade: (db) => db.createObjectStore('kept') })).close()

    await retireOrphanedStorage()

    const names = (await indexedDB.databases()).map((d) => d.name)
    expect(names).not.toContain('crowbar-parser-cache')
    expect(names).toContain('crowbar')
  })

  it('is a no-op on a fresh profile', async () => {
    await expect(retireOrphanedStorage()).resolves.toBeUndefined()
    expect(await indexedDB.databases()).toEqual([])
  })
})
