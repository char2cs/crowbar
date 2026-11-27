import { openDB } from 'idb'
import type { IDBPDatabase, IDBPTransaction, StoreNames } from 'idb'
import type { CrowbarDB } from './schemas'

const connections = new WeakMap<IDBFactory, Promise<IDBPDatabase<CrowbarDB>>>()

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  const factory = globalThis.indexedDB
  if (!factory) throw new Error('IndexedDB is unavailable')
  const held = connections.get(factory)
  if (held) return held
  const opening: Promise<IDBPDatabase<CrowbarDB>> = openDatabase(() => {
    if (connections.get(factory) === opening) connections.delete(factory)
  })
  connections.set(factory, opening)
  opening.catch(() => connections.delete(factory))
  return opening
}

async function openDatabase(release: () => void): Promise<IDBPDatabase<CrowbarDB>> {
  try {
    return await openDeclared(release)
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

async function openDeclared(release: () => void): Promise<IDBPDatabase<CrowbarDB>> {
  return openDB<CrowbarDB>('crowbar', 16, {
    blocking(_current, _blocked, event) {
      const held = event.target as IDBDatabase
      held.close()
      release()
    },
    terminated: release,
    upgrade(db, oldVersion) {
      const raw = db as unknown as IDBDatabase
      if (oldVersion < 1) {
        db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
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
        db.createObjectStore('crowbar_folders', { keyPath: 'id' })
      }
      if (oldVersion < 9) {
        db.createObjectStore('crowbar_chats', { keyPath: 'id' })
      }
      if (oldVersion < 10) {
        for (const name of ['ui-preferences', 'workspace-hierarchy']) {
          if (raw.objectStoreNames.contains(name)) raw.deleteObjectStore(name)
        }
      }
      if (oldVersion < 12) {
        for (const name of RETIRED_V12) {
          if (raw.objectStoreNames.contains(name)) raw.deleteObjectStore(name)
        }
      }
      if (oldVersion < 14) {
        if (raw.objectStoreNames.contains('editor-state')) raw.deleteObjectStore('editor-state')
        if (!raw.objectStoreNames.contains('editor-drafts')) {
          db.createObjectStore('editor-drafts', { keyPath: 'uri' })
        }
      }
      if (oldVersion < 15) {
        const feeds = db.createObjectStore('chat-feeds', { keyPath: ['hostId', 'chatId'] })
        feeds.createIndex('savedAt', 'savedAt')
        feeds.createIndex('chatId', 'chatId')
      }
      if (oldVersion < 16) {
        db.createObjectStore('host-placements', { keyPath: 'key' }).createIndex('hostId', 'hostId')
      }
    },
  })
}

export async function commit<Stores extends ArrayLike<StoreNames<CrowbarDB>>>(
  tx: IDBPTransaction<CrowbarDB, Stores, 'readwrite'>,
  write: () => Promise<unknown>,
): Promise<void> {
  await Promise.all([tx.done, (async () => write())()])
}
