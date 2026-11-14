import { entityStore } from '@/core/entities/entity-store'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO } from '@/lib/types'

/** `thread` as `wsId`'s ws: scope streams it, the root message flattened onto the thread. */
export function putReviewThread(wsId: string, thread: ReviewThread): void {
  const [root, ...replies] = thread.messages
  const dto: ThreadDTO = {
    id: thread.id,
    projectId: 'p1',
    repoId: 'r1',
    workspaceId: wsId,
    filePath: thread.filePath,
    line: thread.lineNumber,
    startLine: thread.startLine,
    endLine: thread.endLine,
    side: thread.side,
    messageId: root.id,
    body: root.body,
    author: root.author ?? '',
    isAgent: root.isAgent,
    resolved: thread.isResolved,
    createdAt: root.createdAt,
    replies: replies.map((r) => ({ ...r, threadId: thread.id, author: r.author ?? '' })),
  }
  const { byKind } = entityStore.getState()
  entityStore.setState({
    byKind: {
      ...byKind,
      review_thread: { ...byKind.review_thread, [dto.id]: { v: 1, data: dto } },
    },
  })
}

/** No review threads, in any workspace. */
export function clearReviewThreads(): void {
  const { byKind } = entityStore.getState()
  entityStore.setState({ byKind: { ...byKind, review_thread: {} } })
}
