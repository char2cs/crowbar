import { useState } from 'react'
import { nanoid } from 'nanoid'
import { ChatCircle } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ScrollArea } from '@/components/ui/scroll-area'
import { toast } from '@/features/window/stores/toast-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'
import { openThread, replyToThread, setThreadResolved } from '../api/review-api'
import { ReviewThreadItem } from './review-thread-item'

interface ReviewThreadPanelProps {
  wsId: string
}

const NOW = () => new Date().toISOString()

export function ReviewThreadPanel({ wsId }: ReviewThreadPanelProps) {
  const threads = useWorkspaceStoreContext((s) => s.branchReview.threads)
  const store = useWorkspaceStore()

  const [filePath, setFilePath] = useState('')
  const [lineNumber, setLineNumber] = useState('')
  const [body, setBody] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleOpenThread = async () => {
    const trimmedPath = filePath.trim()
    const trimmedBody = body.trim()
    const line = Number.parseInt(lineNumber, 10)
    if (!trimmedPath || !trimmedBody || Number.isNaN(line) || isSubmitting) return

    setIsSubmitting(true)

    // Optimistic insert with a temporary id; reconcile/rollback on the server
    // result. Stores never import components, so the toast lives here.
    const tempId = `temp-${nanoid()}`
    const optimistic: ReviewThread = {
      id: tempId,
      filePath: trimmedPath,
      lineNumber: line,
      startLine: line,
      endLine: line,
      side: 'new',
      isResolved: false,
      messages: [
        {
          id: `temp-${nanoid()}`,
          author: null,
          isAgent: false,
          body: trimmedBody,
          createdAt: NOW(),
        },
      ],
    }
    const actions = store.getState()
    actions.addReviewThread(optimistic)

    try {
      const created = await openThread(wsId, {
        filePath: trimmedPath,
        line,
        startLine: line,
        endLine: line,
        side: 'new',
        body: trimmedBody,
      })
      // Swap the optimistic placeholder for the server thread.
      store.getState().removeReviewThread(tempId)
      store.getState().addReviewThread(created)
      setFilePath('')
      setLineNumber('')
      setBody('')
    } catch {
      store.getState().removeReviewThread(tempId)
      toast.error('Failed to open review thread')
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleReply = async (threadId: string, replyBody: string) => {
    const tempMessage: ReviewMessage = {
      id: `temp-${nanoid()}`,
      author: null,
      isAgent: false,
      body: replyBody,
      createdAt: NOW(),
    }
    store.getState().addReviewMessage(threadId, tempMessage)
    try {
      await replyToThread(wsId, threadId, replyBody)
    } catch {
      // Roll back the optimistic message by removing then re-adding the thread
      // is heavy-handed; instead surface the failure and let a refresh reconcile.
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
                onReply={handleReply}
                onResolve={handleResolve}
              />
            ))
          )}
        </div>
      </ScrollArea>

      <div className="flex shrink-0 flex-col gap-1.5 border-border border-t p-3">
        <span className="ui-text-xs font-medium text-muted-foreground">New thread</span>
        <div className="flex gap-1.5">
          <Input
            value={filePath}
            onChange={(e) => setFilePath(e.target.value)}
            placeholder="path/to/file.ts"
            className="ui-text-sm flex-1"
          />
          <Input
            value={lineNumber}
            onChange={(e) => setLineNumber(e.target.value.replace(/[^0-9]/g, ''))}
            placeholder="Line"
            inputMode="numeric"
            className="ui-text-sm w-20"
          />
        </div>
        <Textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Start a review comment…"
          rows={2}
          className="ui-text-sm"
        />
        <div className="flex justify-end">
          <Button
            size="sm"
            variant="outline"
            onClick={handleOpenThread}
            disabled={
              isSubmitting ||
              filePath.trim().length === 0 ||
              lineNumber.trim().length === 0 ||
              body.trim().length === 0
            }
          >
            Open thread
          </Button>
        </div>
      </div>
    </div>
  )
}
