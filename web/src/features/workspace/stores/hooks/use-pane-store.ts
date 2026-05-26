import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStoreContext } from '../workspace-context'
import type { PaneActions } from '../slices/pane-slice'
import type { PaneGroup, PaneNode } from '@/features/panes/types/pane'

export const usePaneRoot = (): PaneNode =>
  useWorkspaceStoreContext(s => s.paneRoot)

export const useBottomRoot = (): PaneNode =>
  useWorkspaceStoreContext(s => s.bottomRoot)

export const useFullscreenPaneId = (): string | null =>
  useWorkspaceStoreContext(s => s.fullscreenPaneId)

export const useActivePaneId = (): string =>
  useWorkspaceStoreContext(s => s.activePaneId)

export const useMostRecentActivePaneIds = (): string[] =>
  useWorkspaceStoreContext(s => s.mostRecentActivePaneIds)

export const usePaneActions = (): PaneActions =>
  useWorkspaceStoreContext(s => s.paneActions)

/**
 * Returns all pane groups in the workspace.
 * useShallow prevents re-renders when the returned array has the same elements
 * even though getAllPaneGroups() builds a new array reference each call.
 */
export const useAllPaneGroups = (): PaneGroup[] =>
  useWorkspaceStoreContext(useShallow(s => s.paneActions.getAllPaneGroups()))
