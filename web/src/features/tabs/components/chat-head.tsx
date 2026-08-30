import { Button } from '@/components/ui/button'
import { AgentChatGlyph } from '@/features/agent/shared/agent-chat-glyph'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { cn } from '@/utils/cn'

interface ChatHeadProps {
  chatId: string
  isActive: boolean
  onSelect: () => void
}

/**
 * The pane-top row's leading identity (spec §7.1): the chat is not a tab —
 * no close affordance, no reordering, outside the editor-tab scroller. Reads
 * the chat's title/provider/turn state the same way Recents does
 * (`agentChats` by id, tab-bar-item.tsx's old `AgentChatTabIcon` pattern),
 * so the two surfaces can never show a different name or glyph for the same
 * chat.
 */
export function ChatHead({ chatId, isActive, onSelect }: ChatHeadProps) {
  const title = useWorkspaceStoreContext(
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.title ?? 'Chat',
  )
  const working = useWorkspaceStoreContext((s) => s.agentChats.working[chatId] ?? false)
  const providerIcon = useWorkspaceStoreContext((s) => {
    const chat = s.agentChats.chats.find((c) => c.id === chatId)
    if (!chat) return ''
    return s.agentChats.providers.find((p) => p.id === chat.activeProviderId)?.icon ?? ''
  })

  return (
    <Button
      variant="ghost"
      data-testid="chat-head"
      data-role="chat-head"
      aria-current={isActive ? 'true' : undefined}
      onClick={onSelect}
      className={cn(
        'h-8 shrink-0 gap-1.5 rounded-full px-2.5 text-[13px]',
        isActive ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground',
      )}
    >
      <AgentChatGlyph providerIcon={providerIcon} working={working} className="size-3.5" />
      <span className="max-w-[160px] truncate">{title}</span>
    </Button>
  )
}
