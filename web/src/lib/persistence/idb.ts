import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  if (_db) return _db
  _db = await openDB<CrowbarDB>('crowbar', 7, {
    upgrade(db, oldVersion) {
      if (oldVersion < 1) {
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
        const editorStore = db.createObjectStore('editor-state', {
          keyPath: ['workspaceId', 'bufferId'],
        })
        editorStore.createIndex('workspaceId', 'workspaceId')
        db.createObjectStore('ui-preferences')
        // query-cache was removed in v5; created here only for upgrade path completeness
        ;(db as unknown as IDBDatabase).createObjectStore('query-cache')
      }
      if (oldVersion < 2) {
        db.deleteObjectStore('workspace-layout')
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
      }
      if (oldVersion < 3) {
        db.createObjectStore('sidebar-ui')
        db.createObjectStore('workspace-hierarchy', { keyPath: 'repoId' })
      }
      if (oldVersion < 4) {
        db.createObjectStore('branch-review', { keyPath: 'wsId' })
      }
      if (oldVersion < 5) {
        if ((db as unknown as IDBDatabase).objectStoreNames.contains('query-cache')) {
          ;(db as unknown as IDBDatabase).deleteObjectStore('query-cache')
        }
        for (const name of [
          'workspaces-data',
          'git-data',
          'file-tree-data',
          'branch-review-data',
          'chat-history',
          'projects-data',
        ] as const) {
          db.createObjectStore(name, { keyPath: 'key' })
        }
      }
      if (oldVersion < 6) {
        db.createObjectStore('chats-data', { keyPath: 'key' })
      }
      if (oldVersion < 7) {
        // §6 entity cache: complete DTOs keyed by their own id (additive).
        for (const name of [
          'crowbar_projects',
          'crowbar_repos',
          'crowbar_workspaces',
          'crowbar_threads',
        ] as const) {
          db.createObjectStore(name, { keyPath: 'id' })
        }
      }
    },
  })
  return _db
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
}

// ---------------------------------------------------------------------------
// §6 version-gated wipe. Pre-production there are no users and no migrations:
// on a daemon/cache version change we drop the entity cache wholesale and let
// the app re-seed from GET. Bump CROWBAR_CACHE_VERSION whenever the DTO shape
// or the daemon's persisted model changes incompatibly.
// ---------------------------------------------------------------------------

export const CROWBAR_CACHE_VERSION = '1'

const CACHE_VERSION_KEY = 'crowbar:cache-version'

const ENTITY_STORES = [
  'crowbar_projects',
  'crowbar_repos',
  'crowbar_workspaces',
  'crowbar_threads',
] as const

/** Clears the four crowbar_* entity stores. Best-effort: IDB failures no-op. */
export async function wipeEntityCache(): Promise<void> {
  try {
    const db = await getDB()
    const tx = db.transaction(ENTITY_STORES, 'readwrite')
    await Promise.all(ENTITY_STORES.map((name) => tx.objectStore(name).clear()))
    await tx.done
  } catch {
    /* best-effort wipe — ignore IDB failures */
  }
}

/**
 * Compares the persisted cache version (localStorage) to CROWBAR_CACHE_VERSION.
 * On a mismatch (including first run) wipe the entity cache and record the new
 * version; on a match this is a no-op. Call once at startup before seeding.
 */
export async function maybeWipeOnVersionChange(): Promise<void> {
  let stored: string | null = null
  try {
    stored = localStorage.getItem(CACHE_VERSION_KEY)
  } catch {
    stored = null
  }
  if (stored === CROWBAR_CACHE_VERSION) return
  await wipeEntityCache()
  try {
    localStorage.setItem(CACHE_VERSION_KEY, CROWBAR_CACHE_VERSION)
  } catch {
    /* localStorage unavailable (private mode) — wipe still ran */
  }
}
