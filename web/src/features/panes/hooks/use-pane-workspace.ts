import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { PaneGroup } from '@/features/panes/types/pane'

/**
 * The workspace a pane's chat belongs to — recorded on the pane by the gesture
 * that put the chat there (C3), so it answers on the first render — not the
 * ambient one whose view happens to render the pane. A chatless pane uses the
 * ambient workspace.
 *
 * `chatStore` never mints a store: a workspace `WorkspaceHost` did not mount
 * falls back to the ambient store, which (chat lists being repo-scoped) still
 * knows every chat of its own repo.
 */
export function usePaneWorkspace(pane: Pick<PaneGroup, 'chatId' | 'workspaceId'>) {
  const ambientWsId = useWorkspaceStoreContext((s) => s.workspaceId)
  const ambientStore = useWorkspaceStore()
  const chatWsId = pane.chatId ? (pane.workspaceId ?? null) : null
  const wsId = chatWsId ?? ambientWsId
  const chatStore = (chatWsId && getWorkspaceStore(chatWsId)) || ambientStore
  return { wsId, chatWsId, chatStore }
}
