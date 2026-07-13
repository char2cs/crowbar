import { useStore } from 'zustand'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { AgentChatGlyph } from './agent-chat-glyph'

interface AgentChatTabIconProps {
  wsId: string
  chatId: string
}

/**
 * The pane tab's glyph for an agent chat: the chat's provider, or the spinner
 * while its agent is mid-turn.
 *
 * It subscribes to the workspace store itself rather than taking props, because
 * TabBar renders from the BUFFER list, which carries no provider or turn state —
 * threading them through every tab would couple the whole tab bar to the agent
 * feature. Both selectors return primitives, so a tab only re-renders when its
 * own chat's provider or turn state actually changes.
 *
 * A working chat is tinted brighter than an idle one: a tab that is thinking is
 * the one thing in the strip worth the user's eye.
 */
export function AgentChatTabIcon({ wsId, chatId }: AgentChatTabIconProps) {
  const store = getOrCreateWorkspaceStore(wsId)

  const working = useStore(store, (s) => s.agentChats.working[chatId] ?? false)
  const providerIcon = useStore(store, (s) => {
    const chat = s.agentChats.chats.find((c) => c.id === chatId)
    if (!chat) return ''
    return s.agentChats.providers.find((p) => p.id === chat.activeProviderId)?.icon ?? ''
  })

  return (
    <AgentChatGlyph
      providerIcon={providerIcon}
      working={working}
      className={working ? 'size-3.5 text-foreground' : 'size-3.5 text-muted-foreground'}
    />
  )
}
