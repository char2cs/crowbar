import { useStore } from 'zustand'
import { entityStore } from '@/core/entities/entity-store'
import { selectReviewThreads } from '@/core/entities/select'
import { mapThread } from '@/features/git/api/review-api'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO } from '@/lib/types'

// One mapped array per selected one, so every reader of a workspace shares it.
const mapped = new WeakMap<ThreadDTO[], ReviewThread[]>()

function threadsOf(dtos: ThreadDTO[]): ReviewThread[] {
  let threads = mapped.get(dtos)
  if (!threads) mapped.set(dtos, (threads = dtos.map(mapThread)))
  return threads
}

/** The review threads of `wsId`, kept live by the workspace's ws: scope. */
export function useReviewThreads(wsId: string): ReviewThread[] {
  return useStore(entityStore, (s) => threadsOf(selectReviewThreads(s, wsId)))
}
