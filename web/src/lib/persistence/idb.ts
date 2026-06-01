import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  if (_db) return _db
  _db = await openDB<CrowbarDB>('crowbar', 3, {
    upgrade(db, oldVersion) {
      if (oldVersion < 1) {
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
        const editorStore = db.createObjectStore('editor-state', {
          keyPath: ['workspaceId', 'bufferId'],
        })
        editorStore.createIndex('workspaceId', 'workspaceId')
        db.createObjectStore('ui-preferences')
        db.createObjectStore('query-cache')
      }
      if (oldVersion < 2) {
        db.deleteObjectStore('workspace-layout')
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
      }
      if (oldVersion < 3) {
        db.createObjectStore('sidebar-ui')
        db.createObjectStore('workspace-hierarchy', { keyPath: 'repoId' })
      }
    },
  })
  return _db
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
}
