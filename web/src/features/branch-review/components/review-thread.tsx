import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

interface ReviewThreadViewProps {
  thread: ReviewThread
  onReply: (body: string) => void
  onResolve: () => void
}

export function ReviewThreadView({ thread, onReply, onResolve }: ReviewThreadViewProps) {
  const [replyText, setReplyText] = useState('')
  const [collapsed, setCollapsed] = useState(false)

  if (thread.isResolved && collapsed) {
    return (
      <div className="my-1 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground/40">
        <span>Thread resolved</span>
        <button className="underline" onClick={() => setCollapsed(false)}>Show</button>
      </div>
    )
  }

  const handleReply = () => {
    const trimmed = replyText.trim()
    if (!trimmed) return
    onReply(trimmed)
    setReplyText('')
  }

  return (
    <div className={cn('my-1 rounded-lg border border-border bg-muted/20', thread.isResolved && 'opacity-60')}>
      {thread.messages.map(msg => (
        <div key={msg.id} className="border-b border-border/40 last:border-b-0 px-3 py-2">
          <div className="mb-1 flex items-center gap-1.5">
            <span className="text-[11px] font-semibold text-foreground">{msg.author ?? 'You'}</span>
            {msg.isAgent && (
              <Badge variant="outline" className="h-3.5 px-1 text-[9px] text-blue-400 border-blue-400/30">
                agent
              </Badge>
            )}
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">{msg.body}</p>
        </div>
      ))}
      <div className="flex items-center gap-2 px-3 py-2">
        <input
          className="flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground/30"
          placeholder="Reply..."
          value={replyText}
          onChange={e => setReplyText(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              handleReply()
            }
          }}
        />
        {!thread.isResolved ? (
          <Button size="sm" variant="ghost" className="h-6 px-2 text-[10px]" onClick={onResolve}>
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
    </div>
  )
}
