import { useSyncExternalStore } from 'react'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import {
  getActiveWorkspaceId,
  getWorkspaceStore,
  subscribeActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { PaneGroup } from '@/features/panes/types/pane'

/**
 * The workspace a pane belongs to — recorded on the pane by the gesture that put
 * its chat there (C3), so it answers on the first render. A chatless pane
 * (editor tabs only) follows the ACTIVE workspace.
 *
 * Only a chatless pane subscribes to the active workspace: the ambient context
 * store is deliberately stable, so a focus click that switches workspace
 * re-renders the panes it moves between, not every pane in the window.
 *
 * `chatStore` never mints a store: a workspace `WorkspaceHost` did not mount
 * falls back to the active (then the ambient) store, which (chat lists being
 * repo-scoped) still knows every chat of its own repo.
 */
export function usePaneWorkspace(pane: Pick<PaneGroup, 'chatId' | 'workspaceId'>) {
  const ambientStore = useWorkspaceStore()
  const chatWsId = pane.chatId ? (pane.workspaceId ?? null) : null
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
