import { useWorkspaceStoreContext } from '../workspace-context'
import type { PaneActions } from '../slices/pane-slice'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'

export const useRootLayout = (): LayoutNode => useWorkspaceStoreContext((s) => s.rootLayout)

export const useBottomLayout = (): LayoutNode => useWorkspaceStoreContext((s) => s.bottomLayout)

export const usePanes = (): Record<string, PaneGroup> => useWorkspaceStoreContext((s) => s.panes)

export const useFullscreenPaneId = (): string | null =>
  useWorkspaceStoreContext((s) => s.fullscreenPaneId)

export const useActivePaneId = (): string => useWorkspaceStoreContext((s) => s.activePaneId)

export const useMostRecentActivePaneIds = (): string[] =>
  useWorkspaceStoreContext((s) => s.mostRecentActivePaneIds)

export const usePaneActions = (): PaneActions => useWorkspaceStoreContext((s) => s.paneActions)

export const usePaneById = (paneId: string): PaneGroup | null =>
  useWorkspaceStoreContext((s) => s.panes[paneId] ?? null)
