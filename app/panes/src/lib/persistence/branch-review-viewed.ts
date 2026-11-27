import { getDB } from './idb'

export async function loadBranchReviewViewed(wsId: string): Promise<Record<string, string> | null> {
  const db = await getDB()
  const row = await db.get('branch-review', wsId)
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
