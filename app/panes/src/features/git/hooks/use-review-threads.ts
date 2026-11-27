import { useStore } from 'zustand'
import { entityStore } from '@/core/entities/entity-store'
import { selectReviewThreads } from '@/core/entities/select'
import { mapThread } from '@/features/git/api/review-api'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO } from '@/lib/types'

const mapped = new WeakMap<ThreadDTO[], ReviewThread[]>()

function threadsOf(dtos: ThreadDTO[]): ReviewThread[] {
  let threads = mapped.get(dtos)
  if (!threads) mapped.set(dtos, (threads = dtos.map(mapThread)))
  return threads
}

export function useReviewThreads(wsId: string): ReviewThread[] {
  return useStore(entityStore, (s) => threadsOf(selectReviewThreads(s, wsId)))
}
