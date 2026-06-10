import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  if (_db) return _db
  _db = await openDB<CrowbarDB>('crowbar', 6, {
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
    },
  })
  return _db
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
}
