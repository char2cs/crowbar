import { useMemo, useCallback, useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

// Ids can never contain NUL (workspace-host.tsx's own guarantee, reused
// verbatim here) — safe as a join delimiter for the stable keys below.
const ID_DELIM = '\x00'

/**
 * The workspace the ACTIVE PANE's chat belongs to — read off the pane record
 * (C3), so clicking between two chats of the same workspace re-renders
 * nothing: the selector's answer does not move.
 */
export function useActivePaneWorkspaceId(): string | null {
  return useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.workspaceId ?? null)
}

/**
 * Every workspace id some pane's EDITOR TABS reference — files, terminals,
 * diffs, previews — via each open buffer's own `workspaceId`
 * (`EditorTabBase.workspaceId`, pane-content.ts).
 *
 * `useViewWorkspaceIds` below only names a pane's CHAT — but a pane can
 * hold editor tabs with `chatId: null` (an editor-only split, e.g. a file
 * opened beside a chat pane). Such a pane names no chat at all, so it was
 * invisible to `WorkspaceHost`'s retention set (`paneWsIds`): unless its
 * workspace also happened to be the single active one, or own a chat
 * Recents was tracking, `planRetention` (keep-alive-policy.ts) could
 * legitimately evict it — destroying the workspace's store, and with it
 * `EditorSurface`'s `editorManager` — while the pane displaying its file was
 * still on screen. Live-reported as "Editor failed to load. Try closing and
 * reopening this file.": the split's OTHER pane (a chat) switched the
 * active workspace elsewhere, its own workspace had no chat left in
 * Recents, and the next render's `getWorkspaceStore(workspaceId)!.editorManager`
 * threw on the now-destroyed store.
 */
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

/**
 * Every workspace owning a chat held by some view record — "in a view" per
 * `keep-alive-policy.ts`. Read straight off the members' records: no scan of
 * the workspace stores, and the string key means a pane-store write that
 * moves no workspace (every streaming frame) re-renders nothing.
 */
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
