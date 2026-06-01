import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/utils/cn'

interface AboutTabProps {
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const chats = useSidebarStore(s => s.chats)

  return (
    <div className="flex flex-col gap-6 p-5">
      <div className="flex flex-col gap-2">
        <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">Description</p>
        <textarea
          value={description}
          onChange={e => onDescriptionChange(e.target.value)}
          placeholder="Describe what this branch does, its goals, and any context needed for review…"
          rows={5}
          className="w-full resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground/40 outline-none leading-relaxed"
        />
      </div>

      <div className="flex flex-col gap-2">
        <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">Conversations</p>
        {chats.length === 0 ? (
          <p className="text-xs text-muted-foreground/40">No conversations yet.</p>
        ) : (
          <div className="flex flex-col">
            {chats.map(chat => (
              <button
                key={chat.id}
                onClick={() => onOpenConversation(chat.id)}
                className="flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-accent"
              >
                <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full',
                  chat.age === 'active' ? 'bg-green-500' : 'bg-muted-foreground/30')} />
                <span className="flex-1 truncate text-sm text-foreground">{chat.title}</span>
                <span className="text-xs text-muted-foreground/50">{chat.age}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
