import { useState } from 'react'
import { CheckCircle, ArrowCounterClockwise } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import { MarkdownPreview } from '@/features/panes/lib/markdown'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

export interface ReviewThreadItemProps {
  thread: ReviewThread
  wsId: string
  isOutdated?: boolean
  onReply: (threadId: string, body: string) => Promise<void>
  onResolve: (threadId: string) => Promise<void>
  onReopen: (threadId: string) => Promise<void>
}

function MessageRow({ message }: { message: ReviewMessage }) {
  const initials = (message.author ?? 'U').slice(0, 2).toUpperCase()
  return (
    <div className="flex gap-2 px-3 py-2 border-b border-border/40 last:border-b-0">
      <div className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[9px] font-semibold text-muted-foreground">
        {initials}
      </div>
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-center gap-1.5">
          <span className="text-[11px] font-semibold text-foreground">
            {message.author ?? (message.isAgent ? 'Agent' : 'You')}
          </span>
          {message.isAgent && (
            <Badge variant="outline" className="h-3.5 border-primary/30 px-1 text-[9px] text-primary">
              agent
            </Badge>
          )}
        </div>
        <MarkdownPreview className="text-xs">{message.body}</MarkdownPreview>
      </div>
    </div>
  )
}

export function ReviewThreadItem({
  thread,
  isOutdated,
  onReply,
  onResolve,
  onReopen,
}: ReviewThreadItemProps) {
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isReplying, setIsReplying] = useState(false)
  const [outdatedExpanded, setOutdatedExpanded] = useState(false)

  // Show outdated collapsed state
  if (isOutdated && !outdatedExpanded) {
    return (
      <div className="my-1 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground/40">
        <Badge variant="outline" className="h-3.5 border-border/40 px-1 text-[9px] text-muted-foreground/60">
          Outdated
        </Badge>
        <button className="underline" onClick={() => setOutdatedExpanded(true)}>
          Show
        </button>
      </div>
    )
  }

  const handleReply = async (body: string) => {
    if (isSubmitting) return
    setIsSubmitting(true)
    try {
      await onReply(thread.id, body)
      setIsReplying(false)
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

  const handleReopen = async () => {
    if (isSubmitting || !thread.isResolved) return
    setIsSubmitting(true)
    try {
      await onReopen(thread.id)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div
      className={cn(
        'my-1 rounded-lg border border-border bg-muted/20',
        thread.isResolved && 'opacity-60',
        isOutdated && outdatedExpanded && 'border-border/40',
      )}
    >
      {isOutdated && (
        <div className="flex items-center gap-1.5 border-b border-border/40 px-3 py-1">
          <Badge variant="outline" className="h-3.5 border-border/40 px-1 text-[9px] text-muted-foreground/60">
            Outdated
          </Badge>
        </div>
      )}

      {/* Messages */}
      <div className="flex flex-col">
        {thread.messages.map((message) => (
          <MessageRow key={message.id} message={message} />
        ))}
      </div>

      {/* Actions */}
      <div className="px-3 py-2">
        {isReplying ? (
          <CommentComposer
            placeholder="Reply…"
            submitLabel="Reply"
            onSubmit={handleReply}
            onCancel={() => setIsReplying(false)}
          />
        ) : (
          <div className="flex items-center justify-between gap-2">
            {!thread.isResolved && (
              <button
                className="flex-1 rounded-md border border-border/60 bg-transparent px-3 py-1.5 text-left text-xs text-muted-foreground/50 hover:border-border"
                onClick={() => setIsReplying(true)}
              >
                Reply…
              </button>
            )}
            {thread.isResolved ? (
              <Button
                size="xs"
                variant="ghost"
                onClick={handleReopen}
                disabled={isSubmitting}
              >
                <ArrowCounterClockwise className="size-3.5" />
                Reopen
              </Button>
            ) : (
              <Button
                size="xs"
                variant="ghost"
                onClick={handleResolve}
                disabled={isSubmitting}
              >
                <CheckCircle className="size-3.5" />
                Resolve
              </Button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
