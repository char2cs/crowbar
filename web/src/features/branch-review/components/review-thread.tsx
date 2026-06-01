import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'
import { CommentComposer } from './comment-composer'
import { MarkdownPreview } from '../lib/markdown'

interface ReviewThreadViewProps {
  thread: ReviewThread
  onReply: (body: string) => void
  onResolve: () => void
  /** Discard the whole thread (used to cancel an unsent draft). */
  onDelete: () => void
}

export function ReviewThreadView({ thread, onReply, onResolve, onDelete }: ReviewThreadViewProps) {
  const [collapsed, setCollapsed] = useState(false)
  const [replying, setReplying] = useState(false)

  const isDraft = thread.messages.length === 0
  const lineLabel = `${thread.side === 'left' ? 'L' : 'R'}${thread.lineNumber}`

  // An unsent draft is just the composer. Cancel/Esc discards the thread.
  if (isDraft) {
    return (
      <div className="my-1">
        <CommentComposer
          title={`Add a comment on line ${lineLabel}`}
          onSubmit={onReply}
          onCancel={onDelete}
        />
      </div>
    )
  }

  if (thread.isResolved && collapsed) {
    return (
      <div className="my-1 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground/40">
        <span>Thread resolved</span>
        <button className="underline" onClick={() => setCollapsed(false)}>Show</button>
      </div>
    )
  }

  return (
    <div className={cn('my-1 rounded-lg border border-border bg-muted/20', thread.isResolved && 'opacity-60')}>
      {thread.messages.map(msg => (
        <div key={msg.id} className="border-b border-border/40 px-3 py-2 last:border-b-0">
          <div className="mb-1 flex items-center gap-1.5">
            <span className="text-[11px] font-semibold text-foreground">{msg.author ?? 'You'}</span>
            {msg.isAgent && (
              <Badge variant="outline" className="h-3.5 border-blue-400/30 px-1 text-[9px] text-blue-400">
                agent
              </Badge>
            )}
          </div>
          <MarkdownPreview className="text-xs">{msg.body}</MarkdownPreview>
        </div>
      ))}

      <div className="px-3 py-2">
        {replying ? (
          <CommentComposer
            placeholder="Reply…"
            submitLabel="Reply"
            onSubmit={body => { onReply(body); setReplying(false) }}
            onCancel={() => setReplying(false)}
          />
        ) : (
          <div className="flex items-center justify-between gap-2">
            <button
              className="flex-1 rounded-md border border-border/60 bg-transparent px-3 py-1.5 text-left text-xs text-muted-foreground/50 hover:border-border"
              onClick={() => setReplying(true)}
            >
              Reply…
            </button>
            {!thread.isResolved ? (
              <Button size="sm" variant="ghost" className="h-7 px-2 text-[11px]" onClick={onResolve}>
                Resolve
              </Button>
            ) : (
              <button
                className="text-[10px] text-muted-foreground/40 underline"
                onClick={() => setCollapsed(true)}
              >
                Collapse
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
