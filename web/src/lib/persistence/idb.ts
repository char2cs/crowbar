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
  _opening ??= openDatabase()
  try {
    _db = await _opening
    return _db
  } finally {
    _opening = null
  }
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
  return openDB<CrowbarDB>('crowbar', 11, {
    upgrade(db, oldVersion) {
      // Retired stores are no longer in CrowbarDB; the upgrade chain still
      // creates and then drops them, so it goes through the untyped handle.
      const raw = db as unknown as IDBDatabase
      if (oldVersion < 1) {
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
        // editor-state (dropped in v11), ui-preferences (dropped in v10) and
        // query-cache (dropped in v5) are created only to keep the chain whole.
        raw
          .createObjectStore('editor-state', { keyPath: ['workspaceId', 'bufferId'] })
          .createIndex('workspaceId', 'workspaceId')
        raw.createObjectStore('ui-preferences')
        raw.createObjectStore('query-cache')
      }
      if (oldVersion < 2) {
        db.deleteObjectStore('workspace-layout')
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
      }
      if (oldVersion < 3) {
        db.createObjectStore('sidebar-ui')
        raw.createObjectStore('workspace-hierarchy', { keyPath: 'repoId' })
      }
      if (oldVersion < 4) {
        db.createObjectStore('branch-review', { keyPath: 'wsId' })
      }
      if (oldVersion < 5) {
        if (raw.objectStoreNames.contains('query-cache')) raw.deleteObjectStore('query-cache')
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
      if (oldVersion < 9) {
        // Sidebar chat rows, for exactly the reason above: an object store is
        // only created inside an upgrade, so this needs its own version rather
        // than riding v8's branch (an install already at v8 would never run it).
        db.createObjectStore('crowbar_chats', { keyPath: 'id' })
      }
      if (oldVersion < 10) {
        // Retired: nothing reads ui-preferences (settings persist through the
        // settings store) or workspace-hierarchy (the daemon owns parents).
        for (const name of ['ui-preferences', 'workspace-hierarchy']) {
          if (raw.objectStoreNames.contains(name)) raw.deleteObjectStore(name)
        }
      }
      if (oldVersion < 11) {
        // Documents keep their unsaved text as drafts; nothing used the cursor state.
        if (raw.objectStoreNames.contains('editor-state')) raw.deleteObjectStore('editor-state')
        db.createObjectStore('editor-drafts', { keyPath: 'uri' })
      }
    },
  })
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
  _opening = null
}
