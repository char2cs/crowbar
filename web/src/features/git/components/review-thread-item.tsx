import { useState } from 'react'
import { CheckCircle, Robot, User } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/utils/cn'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

interface ReviewThreadItemProps {
  thread: ReviewThread
  onReply: (threadId: string, body: string) => Promise<void>
  onResolve: (threadId: string) => Promise<void>
}

function MessageRow({ message }: { message: ReviewMessage }) {
  return (
    <div className="flex gap-2">
      <span className="mt-0.5 shrink-0 text-muted-foreground">
        {message.isAgent ? <Robot className="size-4" /> : <User className="size-4" />}
      </span>
      <div className="min-w-0 flex-1">
        <div className="ui-text-xs text-muted-foreground">
          {message.author ?? (message.isAgent ? 'Agent' : 'You')}
        </div>
        <div className="ui-text-sm whitespace-pre-wrap break-words text-foreground">
          {message.body}
        </div>
      </div>
    </div>
  )
}

export function ReviewThreadItem({ thread, onReply, onResolve }: ReviewThreadItemProps) {
  const [replyBody, setReplyBody] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const fileName = thread.filePath.split('/').pop() || thread.filePath

  const handleReply = async () => {
    const body = replyBody.trim()
    if (!body || isSubmitting) return
    setIsSubmitting(true)
    try {
      await onReply(thread.id, body)
      setReplyBody('')
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleResolve = async () => {
    if (isSubmitting || thread.isResolved) return
    setIsSubmitting(true)
    try {
      await onResolve(thread.id)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div
      className={cn(
        'rounded-md border border-border bg-background p-3',
        thread.isResolved && 'opacity-64',
      )}
    >
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="ui-text-xs truncate text-muted-foreground" title={thread.filePath}>
          {fileName}
          <span className="ml-1 editor-font">:{thread.lineNumber}</span>
        </span>
        {thread.isResolved ? (
          <span className="ui-text-xs flex items-center gap-1 text-git-added">
            <CheckCircle className="size-3.5" />
            Resolved
          </span>
        ) : (
          <Button size="xs" variant="ghost" onClick={handleResolve} disabled={isSubmitting}>
            <CheckCircle className="size-3.5" />
            Resolve
          </Button>
        )}
      </div>

      <div className="flex flex-col gap-2">
        {thread.messages.map((message) => (
          <MessageRow key={message.id} message={message} />
        ))}
      </div>

      {!thread.isResolved && (
        <div className="mt-2 flex flex-col gap-1.5">
          <Textarea
            value={replyBody}
            onChange={(e) => setReplyBody(e.target.value)}
            placeholder="Reply…"
            rows={2}
            className="ui-text-sm"
          />
          <div className="flex justify-end">
            <Button
              size="xs"
              variant="outline"
              onClick={handleReply}
              disabled={isSubmitting || replyBody.trim().length === 0}
            >
              Reply
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
