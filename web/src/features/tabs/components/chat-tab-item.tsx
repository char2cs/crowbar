import { Tab } from '@/components/ui/tabs'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { AgentChatGlyph } from '@/features/agent/shared/agent-chat-glyph'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

interface ChatTabItemProps {
  chatId: string
  isActive: boolean
  onSelect: () => void
}

/**
 * The chat's own entry in the IDE sector's tab strip — only rendered in the
 * collapsed ('tabs') presentation, alongside the pane's real editor tabs
 * (see tab-bar.tsx's `showChatTab`). Per the redesign, the chat is "just
 * another tab" you can switch away from here — never closable or
 * reorderable, so it renders outside the sortable list entirely, but
 * otherwise ghost-styled exactly like a real tab (`TabBarItem`) so the two
 * read as the same kind of control.
 *
 * Reads the chat's title/provider/turn state the same way ChatBranchHeader
 * and Recents do — `agentChats` by id — so no surface can ever show a
 * different name or glyph for the same chat.
 */
export function ChatTabItem({ chatId, isActive, onSelect }: ChatTabItemProps) {
  const title = useWorkspaceStoreContext(
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.title || UNTITLED_CHAT_LABEL,
  )
  const working = useWorkspaceStoreContext((s) => s.agentChats.working[chatId] ?? false)
  const providerIcon = useWorkspaceStoreContext((s) => {
    const chat = s.agentChats.chats.find((c) => c.id === chatId)
    if (!chat) return ''
    return s.agentChats.providers.find((p) => p.id === chat.activeProviderId)?.icon ?? ''
  })

  return (
    <Tab
      data-testid="chat-tab-item"
      data-role="chat-tab"
      role="tab"
      aria-selected={isActive}
      onClick={onSelect}
      isActive={isActive}
      variant="ghost"
      className="h-8 shrink-0 gap-1.5 px-2.5 text-[13px]"
    >
      <AgentChatGlyph providerIcon={providerIcon} working={working} className="size-3.5" />
      <span className="max-w-[160px] truncate">{title}</span>
    </Tab>
  )
}
