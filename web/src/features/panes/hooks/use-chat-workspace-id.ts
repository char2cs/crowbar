import { useMemo, useCallback, useSyncExternalStore } from 'react'
import { entityStore } from '@/core/entities/entity-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { paneChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace-id'

// Ids never contain NUL, so it is a safe join delimiter for the stable keys below.
const ID_DELIM = '\x00'

// A pane's chat workspace moves with either store: the pane (focus, open) or the chat (move).
function subscribePanesAndChats(onChange: () => void): () => void {
  const panes = windowPaneStore.subscribe(onChange)
  const chats = entityStore.subscribe(onChange)
  return () => {
    panes()
    chats()
  }
}

function activePaneWorkspaceId(): string | null {
  const { panes, activePaneId } = windowPaneStore.getState()
  return paneChatWorkspaceId(panes[activePaneId])
}

/** The active pane's chat workspace, where the chat runs now: switching between two chats of
 *  one workspace re-renders nothing. */
export function useActivePaneWorkspaceId(): string | null {
  return useSyncExternalStore(subscribePanesAndChats, activePaneWorkspaceId, activePaneWorkspaceId)
}

function viewWorkspaceIdsKey(): string {
  const ids = new Set<string>()
  for (const pane of Object.values(windowPaneStore.getState().panes)) {
    const wsId = pane.viewId ? paneChatWorkspaceId(pane) : null
    if (wsId) ids.add(wsId)
  }
  return [...ids].sort().join(ID_DELIM)
}

/** Every workspace some pane's editor tabs reference. A chatless pane can still hold tabs, which
 *  useViewWorkspaceIds misses; naming them keeps their workspace store retained while shown. */
export function usePaneEditorWorkspaceIds(): string[] {
  const subscribe = useCallback((onChange: () => void) => windowPaneStore.subscribe(onChange), [])
  const snapshot = useCallback(() => {
    const { panes, buffers } = windowPaneStore.getState()
    const bufferWorkspace = new Map(buffers.map((b) => [b.id, b.workspaceId]))
    const ids = new Set<string>()
    for (const pane of Object.values(panes)) {
      for (const tabId of pane.editorTabIds) {
        const wsId = bufferWorkspace.get(tabId)
        if (wsId) ids.add(wsId)
      }
    }
    return [...ids].sort().join(ID_DELIM)
  }, [])
  const key = useSyncExternalStore(subscribe, snapshot, snapshot)
  return useMemo(() => (key ? key.split(ID_DELIM) : []), [key])
}

/** Every workspace with a chat in some view ("in a view" for keep-alive-policy.ts). The string
 *  key means a pane write that moves no workspace (every streaming frame) re-renders nothing. */
export function useViewWorkspaceIds(): string[] {
  const key = useSyncExternalStore(subscribePanesAndChats, viewWorkspaceIdsKey, viewWorkspaceIdsKey)
  return useMemo(() => (key ? key.split(ID_DELIM) : []), [key])
}
