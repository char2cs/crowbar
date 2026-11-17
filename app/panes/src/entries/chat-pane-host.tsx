import { useEffect } from 'react'
import { useStore } from 'zustand'
import { ChatHeaderSlotProvider } from '@/features/agent/hooks/chat-header-slot'
import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { PaneClient } from '@/sdk/pane-client'
import { ChatPaneHeader } from './chat-pane-header'
import { chatSession } from './chat-session'

/** The shell's chat pane: the chat the shell opened, placed in its own workspace like the desktop pane container does, with its identity row and title sent to the shell tab. An untitled chat keeps the shell's own label. */
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
      <ChatHeaderSlotProvider>
        <div className="relative flex h-full min-h-0 w-full flex-col overflow-hidden">
          <ChatPaneHeader chatId={chatId} wsId={wsId} />
          <AgentChatPaneGate
            chatId={chatId}
            runnerId=""
            wsId={wsId}
            paneId={ROOT_PANE_ID}
            isActivePane={active}
            isVisible
            belowOverlayHeader
          />
        </div>
      </ChatHeaderSlotProvider>
    </WorkspaceStoreContext.Provider>
  )
}
