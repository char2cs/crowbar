import { getDB } from './idb'

/** The workspace's viewed map, or null when none was saved or the row predates it. */
export async function loadBranchReviewViewed(wsId: string): Promise<Record<string, string> | null> {
  const db = await getDB()
  const row = await db.get('branch-review', wsId)
  // Rows written before the reshape carried other fields and no `viewed`.
  const viewed: unknown = row?.viewed
  if (viewed == null || typeof viewed !== 'object') return null
  return viewed as Record<string, string>
}

export async function saveBranchReviewViewed(
  wsId: string,
  viewed: Record<string, string>,
): Promise<void> {
  const db = await getDB()
  await db.put('branch-review', { wsId, viewed })
}
