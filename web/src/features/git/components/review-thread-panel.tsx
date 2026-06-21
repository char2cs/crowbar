import { ChatCircle } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { toast } from '@/features/window/stores/toast-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import type { ReviewMessage } from '@/features/workspace/stores/slices/branch-review-slice'
import { replyToThread, setThreadResolved } from '../api/review-api'
import { ReviewThreadItem } from './review-thread-item'

interface ReviewThreadPanelProps {
  wsId: string
}

export function ReviewThreadPanel({ wsId }: ReviewThreadPanelProps) {
  const threads = useWorkspaceStoreContext((s) => s.branchReview.threads)
  const store = useWorkspaceStore()

  const handleReply = async (threadId: string, replyBody: string) => {
    const tempMessage: ReviewMessage = {
      id: `temp-${crypto.randomUUID()}`,
      author: null,
      isAgent: false,
      body: replyBody,
      createdAt: new Date().toISOString(),
    }
    store.getState().addReviewMessage(threadId, tempMessage)
    try {
      await replyToThread(wsId, threadId, replyBody)
    } catch {
      toast.error('Failed to post reply')
      throw new Error('reply failed')
    }
  }

  const handleResolve = async (threadId: string) => {
    store.getState().resolveReviewThread(threadId)
    try {
      await setThreadResolved(wsId, threadId, true)
    } catch {
      toast.error('Failed to resolve thread')
      throw new Error('resolve failed')
    }
  }

  const handleReopen = async (threadId: string) => {
    store.getState().setReviewThreadResolved(threadId, false)
    try {
      await setThreadResolved(wsId, threadId, false)
    } catch {
      toast.error('Failed to reopen thread')
      throw new Error('reopen failed')
    }
  }

  return (
    <div className="flex h-full flex-col">
      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-2 p-3">
          {threads.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-8 text-center text-muted-foreground">
              <ChatCircle className="size-6" />
              <span className="ui-text-sm">No review threads yet.</span>
            </div>
          ) : (
            threads.map((thread) => (
              <ReviewThreadItem
                key={thread.id}
                thread={thread}
                wsId={wsId}
                onReply={handleReply}
                onResolve={handleResolve}
                onReopen={handleReopen}
              />
            ))
          )}
        </div>
      </ScrollArea>
    </div>
  )
}
