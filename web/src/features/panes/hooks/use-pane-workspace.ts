import { useSyncExternalStore } from 'react'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import {
  getActiveWorkspaceId,
  getWorkspaceStore,
  subscribeActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { PaneGroup } from '@/features/panes/types/pane'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'

/** A pane's workspace: its chat's own, which a move changes, else its record's (C3); only a
 *  chatless pane follows (and subscribes to) the active workspace, so a focus switch re-renders
 *  just those. `chatStore` never mints: unmounted, it falls back to the active then ambient store. */
export function usePaneWorkspace(pane: Pick<PaneGroup, 'chatId' | 'workspaceId'>) {
  const ambientStore = useWorkspaceStore()
  const runsIn = useChatField(pane.chatId ?? '', (c) => c?.workspaceId || null)
  const chatWsId = pane.chatId ? (runsIn ?? pane.workspaceId ?? null) : null
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
