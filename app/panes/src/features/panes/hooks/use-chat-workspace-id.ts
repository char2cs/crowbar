import { useMemo, useCallback, useSyncExternalStore } from 'react'
import { entityStore } from '@/core/entities/entity-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { paneChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace-id'

const ID_DELIM = '\x00'

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

export function useViewWorkspaceIds(): string[] {
  const key = useSyncExternalStore(subscribePanesAndChats, viewWorkspaceIdsKey, viewWorkspaceIdsKey)
  return useMemo(() => (key ? key.split(ID_DELIM) : []), [key])
}
