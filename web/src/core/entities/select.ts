import type { EntityState } from '@/core/entities/entity-store'
import type { ThreadDTO } from '@/lib/types'

type Threads = EntityState['byKind']['review_thread']

// Readers subscribe with this selector, so an unchanged kind must answer the same array.
const byWorkspace = new WeakMap<Threads, Map<string, ThreadDTO[]>>()

/** The review threads of `wsId`, one array per review_thread record identity. */
export function selectReviewThreads(s: EntityState, wsId: string): ThreadDTO[] {
  const threads = s.byKind.review_thread
  let memo = byWorkspace.get(threads)
  if (!memo) byWorkspace.set(threads, (memo = new Map()))
  let selected = memo.get(wsId)
  if (!selected) {
    selected = Object.values(threads)
      .map((held) => held.data)
      .filter((t) => t.workspaceId === wsId)
    memo.set(wsId, selected)
  }
  return selected
}
