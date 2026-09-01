import { Tab } from '@/components/ui/tabs'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { AgentChatGlyph } from '@/features/agent/shared/agent-chat-glyph'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

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
 *
 * Renders through the same `Tab` primitive (`@/components/ui/tabs`,
 * `variant="underline"`) that tab-bar-item.tsx uses for file tabs — flat, no
 * radius, no fill at rest, a 2px `--primary` bottom bar when active (Main.dc
 * .html's `.hitem`/`.hitem.is-on` rule) — so the chat head and file tabs can
 * never drift into two different "this is what you're looking at" treatments.
 */
export function ChatHead({ chatId, isActive, onSelect }: ChatHeadProps) {
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
      data-testid="chat-head"
      data-role="chat-head"
      aria-current={isActive ? 'true' : undefined}
      onClick={onSelect}
      isActive={isActive}
      variant="underline"
      className="h-8 shrink-0 gap-1.5 px-2.5 text-[13px]"
    >
      <AgentChatGlyph providerIcon={providerIcon} working={working} className="size-3.5" />
      <span className="max-w-[160px] truncate">{title}</span>
    </Tab>
  )
}
