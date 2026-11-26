import { useEffect } from 'react'
import { useStore } from 'zustand'
import { ChatHeaderSlotProvider } from '@/features/agent/hooks/chat-header-slot'
import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  detachedWorkspaceStore,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { PaneClient } from '@/sdk/pane-client'
import { ChatPaneHeader } from './chat-pane-header'
import { chatSession } from './chat-session'

const TOGGLE_EDITOR = 'panes.toggleEditor'

/** The shell's chat pane: the chat the shell opened, placed in its own workspace like the desktop pane container does, with its identity row and title sent to the shell tab; the row's open editor button asks the shell to open the editor beside the chat. An untitled chat keeps the shell's own label; a draft's row has no title, as develop's. */
export function ChatPaneHost({ client }: { client: Pick<PaneClient, 'setTitle' | 'runCommand'> }) {
  const chatId = useStore(chatSession, (s) => s.chatId)
  const active = useStore(chatSession, (s) => s.active)
  const fillsGroup = useStore(chatSession, (s) => s.fillsGroup)
  const wsId = usePaneWorkspace(chatId)
  const title = useChatField(chatId ?? '', (c) => c?.title ?? '')

  useEffect(() => {
    if (title) client.setTitle(title)
  }, [client, title])

  useEffect(() => {
    if (wsId) setActiveWorkspaceId(wsId)
  }, [wsId])

  if (!chatId) return null
  return (
    <WorkspaceStoreContext.Provider value={paneStore(wsId)}>
      <ChatHeaderSlotProvider>
        <div className="relative flex h-full min-h-0 w-full flex-col overflow-hidden">
          <ChatPaneHeader
            chatId={chatId}
            wsId={wsId}
            onOpenEditor={fillsGroup ? () => client.runCommand(TOGGLE_EDITOR) : undefined}
          />
          <AgentChatPaneGate
            chatId={chatId}
            runnerId=""
            wsId={wsId ?? ''}
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

function paneStore(wsId: string | null) {
  return wsId ? getOrCreateWorkspaceStore(wsId) : detachedWorkspaceStore()
}

function usePaneWorkspace(chatId: string | null): string | null {
  const paneWsId = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID]?.workspaceId ?? null)
  return useChatField(chatId ?? '', (c) => c?.workspaceId || null) ?? paneWsId
}
