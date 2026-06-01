import { getDB } from './idb'
import type { CrowbarDB, CachedRecord } from './schemas'

type CacheStoreName =
  | 'workspaces-data'
  | 'git-data'
  | 'file-tree-data'
  | 'branch-review-data'
  | 'chat-history'
  | 'projects-data'

export async function saveCache<T>(
  store: CacheStoreName,
  key: string,
  data: T,
  fetchedAt: number = Date.now(),
): Promise<void> {
  const db = await getDB()
  await db.put(store, { key, data, fetchedAt } as CrowbarDB[CacheStoreName]['value'])
}

export async function loadCache<T>(
  store: CacheStoreName,
  key: string,
): Promise<CachedRecord<T> | undefined> {
  const db = await getDB()
  const rec = await db.get(store, key)
  return rec as CachedRecord<T> | undefined
}
