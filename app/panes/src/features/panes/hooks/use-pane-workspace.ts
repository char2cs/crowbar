import { useSyncExternalStore } from 'react'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import {
  getActiveWorkspaceId,
  getWorkspaceStore,
  subscribeActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { PaneGroup } from '@/features/panes/types/pane'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'

export function usePaneWorkspace(pane: Pick<PaneGroup, 'chatId' | 'workspaceId'>) {
  const ambientStore = useWorkspaceStore()
  const chatWsId = usePaneChatWorkspaceId(pane)
  const followedWsId = useSyncExternalStore(subscribeActiveWorkspaceId, () =>
    pane.chatId ? null : getActiveWorkspaceId(),
  )
  const wsId = chatWsId ?? followedWsId ?? ambientStore.getState().workspaceId
  const chatStore =
    (chatWsId && getWorkspaceStore(chatWsId)) ||
    getWorkspaceStore(getActiveWorkspaceId() ?? '') ||
    ambientStore
  return { wsId, chatWsId, chatStore }
}

export function usePaneChatWorkspaceId(pane: Pick<PaneGroup, 'chatId' | 'workspaceId'> | null) {
  const runsIn = useChatField(pane?.chatId ?? '', (c) => c?.workspaceId || null)
  return pane?.chatId ? (runsIn ?? pane.workspaceId ?? null) : null
}
