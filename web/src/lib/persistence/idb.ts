import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null
// The IN-FLIGHT open, not just the settled one. Callers arrive together at
// startup, and memoizing only the resolved handle lets every one of them start
// its own `openDB` — which deadlocks the moment an open has to change the
// version, because a second connection at the old version blocks the upgrade
// and the upgrade holds the second open. One open, awaited by everyone.
let _opening: Promise<IDBPDatabase<CrowbarDB>> | null = null

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  if (_db) return _db
  _opening ??= healEntityStores(openDatabase())
  try {
    _db = await _opening
    return _db
  } finally {
    _opening = null
  }
}

/**
 * Create any entity store the database is missing, by reopening one version up.
 *
 * An object store can only be created inside an `upgrade`, so a database that
 * reached the current version without running the branch that adds a store
 * never gets it — and it cannot get it later, because the version no longer
 * changes. Every entity-cache call is best-effort, so the result is not an
 * error but an empty list forever: the feature backed by that store silently
 * does nothing, with a green typecheck and a green test suite.
 *
 * That skew is reachable whenever more than one build runs against this origin
 * — several checkouts and an installed app share one bundle id, so an older
 * build holding the database open is enough to strand a newer one's upgrade.
 *
 * Healing beats wiping here: the entity stores are re-seedable from GET, but
 * the rest of the database (editor state, layout, chat history) is not, so
 * `deleteDatabase` would cost the user real work to repair a cache. Creating
 * just what is missing costs nothing and leaves everything else untouched.
 */
async function healEntityStores(
  opening: Promise<IDBPDatabase<CrowbarDB>>,
): Promise<IDBPDatabase<CrowbarDB>> {
  const db = await opening
  const missing = ENTITY_STORES.filter((name) => !db.objectStoreNames.contains(name))
  if (missing.length === 0) return db
  const next = db.version + 1
  db.close()
  return openDB<CrowbarDB>('crowbar', next, {
    upgrade(upgraded) {
      // Only the absentees, each guarded: this runs with the real oldVersion, so
      // replaying the version chain above would collide with what already exists.
      for (const name of missing) {
        if (!upgraded.objectStoreNames.contains(name)) {
          upgraded.createObjectStore(name, { keyPath: 'id' })
        }
      }
    },
  })
}

/**
 * Open at the declared version, or as-is when the database is already NEWER.
 *
 * Asking for a version below the one on disk is not a no-op — it throws
 * `VersionError`, and since every cache call is best-effort the app does not
 * crash, it just quietly loses its whole local cache. A newer database is
 * ordinary here: this origin is shared by every build carrying the same bundle
 * id, so running an older checkout after a newer one is a normal Tuesday, and
 * so is a user rolling a release back. The stores are additive, so an older
 * build reading a newer database finds everything it knows about.
 */
async function openDatabase(): Promise<IDBPDatabase<CrowbarDB>> {
  try {
    return await openDeclared()
  } catch (err) {
    if (!(err instanceof DOMException) || err.name !== 'VersionError') throw err
    return openDB<CrowbarDB>('crowbar')
  }
}

async function openDeclared(): Promise<IDBPDatabase<CrowbarDB>> {
  return openDB<CrowbarDB>('crowbar', 8, {
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
      if (oldVersion < 8) {
        // Sidebar grouping folders. This needs its own version: an object store
        // is only created inside an upgrade, so an existing install opened at
        // v7 would simply not have it — and every entity-cache write is
        // best-effort, so the miss would be silent at runtime rather than loud
        // at compile time (an empty folder list forever).
        db.createObjectStore('crowbar_folders', { keyPath: 'id' })
      }
    },
  })
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
  _opening = null
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
  'crowbar_folders',
] as const

/** Clears every crowbar_* entity store. Best-effort: IDB failures no-op. */
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
  let stored: string | null
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
