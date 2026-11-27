import { Tab } from '@/components/ui/tabs'
import { useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { AgentChatGlyph } from '@/features/agent/shared/agent-chat-glyph'
import { useChatField, useChatWorking } from '@/features/agent/hooks/use-chat-entity'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

interface ChatTabItemProps {
  chatId: string
  isActive: boolean
  onSelect: () => void
}

export function ChatTabItem({ chatId, isActive, onSelect }: ChatTabItemProps) {
  const title = useChatTitle(chatId)
  const working = useChatWorking(chatId)
  const providerId = useChatField(chatId, (chat) => chat?.activeProviderId ?? '')
  const providerIcon = useAgentProvidersStore(
    (s) => (providerId && s.providers.find((p) => p.id === providerId)?.icon) || '',
  )

  return (
    <Tab
      data-testid="chat-tab-item"
      data-role="chat-tab"
      data-sidebar-surface="chats"
      role="tab"
      aria-selected={isActive}
      onClick={onSelect}
      isActive={isActive}
      variant="ghost"
      className="h-8 shrink-0 gap-1.5 px-2.5 text-[13px]"
    >
      <AgentChatGlyph providerIcon={providerIcon} working={working} className="size-3.5" />
      <span
        data-testid="chat-tab-item-title"
        data-title-pending={title === null ? 'true' : undefined}
        className="max-w-[160px] truncate"
      >
        {title}
      </span>
    </Tab>
  )
}
