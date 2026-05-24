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

export const useAllPaneGroups = (): PaneGroup[] =>
  useWorkspaceStoreContext(s => s.paneActions.getAllPaneGroups())
