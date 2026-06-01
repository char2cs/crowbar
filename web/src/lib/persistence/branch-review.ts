import { getDB } from './idb'
import type { BranchReviewPersistedState } from './schemas'
import type { ReviewThread, MergeStrategy } from '@/features/branch-review/types/review-types'

interface SavePayload {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  threads: ReviewThread[]
}

export async function saveBranchReview(wsId: string, payload: SavePayload): Promise<void> {
  try {
    const db = await getDB()
    const record: BranchReviewPersistedState = {
      wsId,
      ...payload,
      updatedAt: Date.now(),
    }
    await db.put('branch-review', record)
  } catch {
    // storage unavailable — silently skip
  }
}

export async function loadBranchReview(wsId: string): Promise<Omit<BranchReviewPersistedState, 'wsId' | 'updatedAt'> | null> {
  try {
    const db = await getDB()
    const record = await db.get('branch-review', wsId)
    if (!record) return null
    const { wsId: _wsId, updatedAt: _updatedAt, ...rest } = record
    return rest
  } catch {
    return null
  }
}
