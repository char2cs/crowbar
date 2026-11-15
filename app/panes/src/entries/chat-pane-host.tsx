import { useEffect } from 'react'
import { useStore } from 'zustand'
import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { PaneClient } from '@/sdk/pane-client'
import { chatSession } from './chat-session'

/** The shell's chat pane: the chat the shell opened, placed in its own workspace like the desktop pane container does, with its title sent to the shell tab. An untitled chat keeps the shell's own label. */
export function ChatPaneHost({ client }: { client: Pick<PaneClient, 'setTitle'> }) {
  const chatId = useStore(chatSession, (s) => s.chatId)
  const active = useStore(chatSession, (s) => s.active)
  const wsId = useChatField(chatId ?? '', (c) => c?.workspaceId || null)
  const title = useChatField(chatId ?? '', (c) => c?.title ?? '')

  useEffect(() => {
    if (title) client.setTitle(title)
  }, [client, title])

  useEffect(() => {
    if (wsId) setActiveWorkspaceId(wsId)
  }, [wsId])

  if (!chatId || !wsId) return null
  return (
    <WorkspaceStoreContext.Provider value={getOrCreateWorkspaceStore(wsId)}>
      <AgentChatPaneGate
        chatId={chatId}
        runnerId=""
        wsId={wsId}
        paneId={ROOT_PANE_ID}
        isActivePane={active}
        isVisible
      />
    </WorkspaceStoreContext.Provider>
  )
}
