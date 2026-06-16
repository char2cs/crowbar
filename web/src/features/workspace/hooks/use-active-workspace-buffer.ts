import { useCallback, useSyncExternalStore } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { PaneContent } from '@/features/panes/types/pane-content'

/**
 * Reactively returns the active buffer of the currently-active workspace.
 *
 * The sidebar header lives outside the per-workspace WorkspaceStoreContext
 * provider, so we cannot use `useWorkspaceStoreContext` here. Instead we read
 * the active workspace id from the router and subscribe to that workspace's
 * store from the registry via `useSyncExternalStore`.
 */
export function useActiveWorkspaceBuffer(): PaneContent | null {
  const wsId = useRouterState({
    select: (s) => s.location.pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? '',
  })

  const subscribe = useCallback(
    (onChange: () => void) => {
      if (!wsId) return () => {}
      return getOrCreateWorkspaceStore(wsId).subscribe(onChange)
    },
    [wsId],
  )

  const getSnapshot = useCallback((): PaneContent | null => {
    if (!wsId) return null
    const state = getOrCreateWorkspaceStore(wsId).getState()
    const activeBufferId = state.paneActions.getActivePane()?.activeBufferId
    if (!activeBufferId) return null
    return state.buffers.find((b) => b.id === activeBufferId) ?? null
  }, [wsId])

  return useSyncExternalStore(subscribe, getSnapshot)
}
