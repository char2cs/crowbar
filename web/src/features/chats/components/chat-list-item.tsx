import { cn } from '@/utils/cn'
import { ROW_BASE } from '@/components/layout/workspace-row-base'
import type { ProjectChat } from '@/lib/store/sidebar'

interface ChatListItemProps {
  chat: ProjectChat
  isActive: boolean
  onClick: () => void
}

export function ChatListItem({ chat, isActive, onClick }: ChatListItemProps) {
  return (
    <button
      type="button"
      className={cn(
        ROW_BASE,
        isActive
          ? 'border-background bg-background text-foreground shadow-xs shadow-black/10 not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)]'
          : 'border-transparent text-foreground hover:bg-accent',
      )}
      onClick={onClick}
    >
      <span
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          isActive ? 'bg-success' : 'bg-muted-foreground/40',
        )}
      />
      <span className="min-w-0 flex-1 truncate text-left text-[13px]">{chat.title}</span>
      <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{chat.age}</span>
    </button>
  )
}
