import { getDB } from './idb'

// §6 entity cache: per-entity IndexedDB object stores holding complete DTOs
// keyed by their own `id`. WS frames upsert these by id; a status:'deleted'
// tombstone removes the entity (callers call removeEntity once the delete
// animation completes).
//
// Like cache-store, this layer is strictly best-effort: a missing object store
// (stale schema), a quota error, or a private-mode restriction degrades to a
// no-op / empty read so the caller still functions off the live WS/GET path.
export type EntityStoreName =
  | 'crowbar_projects'
  | 'crowbar_repos'
  | 'crowbar_workspaces'
  | 'crowbar_threads'

export async function upsertEntity<T extends { id: string }>(
  store: EntityStoreName,
  dto: T,
): Promise<void> {
  try {
    const db = await getDB()
    // keyPath is 'id', so no explicit key argument.
    await db.put(store, dto as never)
  } catch {
    /* best-effort cache write — ignore IDB failures */
  }
}

export async function getAllEntities<T>(store: EntityStoreName): Promise<T[]> {
  try {
    const db = await getDB()
    return (await db.getAll(store)) as T[]
  } catch {
    return []
  }
}

export async function removeEntity(store: EntityStoreName, id: string): Promise<void> {
  try {
    const db = await getDB()
    await db.delete(store, id)
  } catch {
    /* best-effort cache delete — ignore IDB failures */
  }
}
