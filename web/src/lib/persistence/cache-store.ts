import { getDB } from './idb'
import type { CrowbarDB, CachedRecord } from './schemas'

export type CacheStoreName =
  | 'workspaces-data'
  | 'git-data'
  | 'file-tree-data'
  | 'branch-review-data'
  | 'chat-history'
  | 'projects-data'

// The cache layer is strictly best-effort: it must NEVER break the data flow.
// A missing object store (stale schema), a quota error, or a private-mode
// restriction degrades to a cache miss / no-op so the caller still fetches and
// renders. (Before this, an IDB read throwing aborted the whole fetch and left
// the loadable stuck on "loading".)

export async function saveCache<T>(
  store: CacheStoreName,
  key: string,
  data: T,
  fetchedAt: number = Date.now(),
): Promise<void> {
  try {
    const db = await getDB()
    await db.put(store, { key, data, fetchedAt } as CrowbarDB[CacheStoreName]['value'])
  } catch {
    /* best-effort cache write — ignore IDB failures */
  }
}

export async function loadCache<T>(
  store: CacheStoreName,
  key: string,
): Promise<CachedRecord<T> | undefined> {
  try {
    const db = await getDB()
    const rec = await db.get(store, key)
    return rec as CachedRecord<T> | undefined
  } catch {
    return undefined
  }
}
