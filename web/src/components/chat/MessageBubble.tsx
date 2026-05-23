import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'

interface MessageBubbleProps {
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
}

export function MessageBubble({
  role,
  content,
  authorName,
  authorInitials,
  modelName,
  timestamp,
}: MessageBubbleProps) {
  const isUser = role === 'user'

  return (
    <div className={cn('flex flex-col px-6 mb-4', isUser ? 'items-end' : 'items-start')}>
      <div
        className={cn(
          'max-w-[75%] rounded-xl px-3.5 py-2.5 text-[13.5px] leading-relaxed',
          isUser
            ? 'rounded-br-sm bg-primary/15 text-primary'
            : 'max-w-[80%] rounded-tl-sm border border-border bg-card text-foreground',
        )}
      >
        {content}
      </div>
      <div className="mt-1.5 flex items-center gap-1.5 text-[10.5px]">
        <Avatar className="h-[17px] w-[17px]">
          <AvatarFallback
            className={cn(
              'text-[7px] font-bold',
              isUser ? 'bg-muted text-muted-foreground' : 'bg-primary text-primary-foreground',
            )}
          >
            {authorInitials}
          </AvatarFallback>
        </Avatar>
        <span className="text-muted-foreground">{authorName}</span>
        {modelName && (
          <>
            <span className="text-muted-foreground/30">·</span>
            <span className="text-primary/70">{modelName}</span>
          </>
        )}
        <span className="text-muted-foreground/30">·</span>
        <span className="text-muted-foreground/50">{timestamp}</span>
      </div>
    </div>
  )
}
