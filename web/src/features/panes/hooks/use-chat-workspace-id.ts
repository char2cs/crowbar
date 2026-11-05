import { useMemo, useCallback, useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

// Ids never contain NUL, so it is a safe join delimiter for the stable keys below.
const ID_DELIM = '\x00'

/** The active pane's chat workspace, off the pane record (C3): switching between two chats of
 *  one workspace re-renders nothing. */
export function useActivePaneWorkspaceId(): string | null {
  return useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.workspaceId ?? null)
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
  const key = useStore(windowPaneStore, (s) => {
    const ids = new Set<string>()
    for (const pane of Object.values(s.panes)) {
      if (pane.viewId && pane.chatId && pane.workspaceId) ids.add(pane.workspaceId)
    }
    return [...ids].sort().join(ID_DELIM)
  })
  return useMemo(() => (key ? key.split(ID_DELIM) : []), [key])
}
