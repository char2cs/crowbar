import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null
// Memoize the in-flight open, not the settled handle: startup callers arrive together, and a
// second `openDB` at the old version blocks a version upgrade that holds it — a deadlock.
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

// Open at the declared version, or as-is when the database is newer: asking below the on-disk
// version throws VersionError and silently loses the cache. Every build with this bundle id shares
// the origin (older checkouts, rollbacks), and stores are additive, so older builds read fine.
async function openDatabase(): Promise<IDBPDatabase<CrowbarDB>> {
  try {
    return await openDeclared()
  } catch (err) {
    if (!(err instanceof DOMException) || err.name !== 'VersionError') throw err
    return openDB<CrowbarDB>('crowbar')
  }
}

const RETIRED_V12 = [
  'git-data',
  'file-tree-data',
  'branch-review-data',
  'chats-data',
  'crowbar_threads',
]

async function openDeclared(): Promise<IDBPDatabase<CrowbarDB>> {
  return openDB<CrowbarDB>('crowbar', 14, {
    upgrade(db, oldVersion) {
      // Retired stores are no longer in CrowbarDB; the upgrade chain still
      // creates and then drops them, so it goes through the untyped handle.
      const raw = db as unknown as IDBDatabase
      if (oldVersion < 1) {
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
        // editor-state (dropped in v14), ui-preferences (dropped in v10) and
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
        ]) {
          raw.createObjectStore(name, { keyPath: 'key' })
        }
      }
      if (oldVersion < 6) {
        raw.createObjectStore('chats-data', { keyPath: 'key' })
      }
      if (oldVersion < 7) {
        // §6 entity cache: complete DTOs keyed by their own id (additive).
        for (const name of [
          'crowbar_projects',
          'crowbar_repos',
          'crowbar_workspaces',
          'crowbar_threads',
        ]) {
          raw.createObjectStore(name, { keyPath: 'id' })
        }
      }
      if (oldVersion < 8) {
        // Sidebar grouping folders get their own version: a store is only created in an upgrade,
        // and best-effort cache writes would otherwise fail silently (empty folder list forever).
        db.createObjectStore('crowbar_folders', { keyPath: 'id' })
      }
      if (oldVersion < 9) {
        // Sidebar chat rows: their own version for the same reason (an install already at v8
        // would never run a branch riding on v8's).
        db.createObjectStore('crowbar_chats', { keyPath: 'id' })
      }
      if (oldVersion < 10) {
        // Retired: nothing reads ui-preferences (settings persist through the
        // settings store) or workspace-hierarchy (the daemon owns parents).
        for (const name of ['ui-preferences', 'workspace-hierarchy']) {
          if (raw.objectStoreNames.contains(name)) raw.deleteObjectStore(name)
        }
      }
      if (oldVersion < 12) {
        // Retired: declared but never read or written by anything.
        for (const name of RETIRED_V12) {
          if (raw.objectStoreNames.contains(name)) raw.deleteObjectStore(name)
        }
      }
      if (oldVersion < 14) {
        // Documents keep their unsaved text as drafts. A database a v11-v13 build upgraded may
        // lack the store (those versions meant other things there), so it is created if missing.
        if (raw.objectStoreNames.contains('editor-state')) raw.deleteObjectStore('editor-state')
        if (!raw.objectStoreNames.contains('editor-drafts')) {
          db.createObjectStore('editor-drafts', { keyPath: 'uri' })
        }
      }
    },
  })
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
  _opening = null
}
