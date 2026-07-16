import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useWorkspaceStoreContext } from '../workspace-context'
import type { PaneActions } from '../slices/pane-slice'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'

export const useRootLayout = (): LayoutNode => useWorkspaceStoreContext((s) => s.rootLayout)

export const useFullscreenPaneId = (): string | null =>
  useWorkspaceStoreContext((s) => s.fullscreenPaneId)

export const useActivePaneId = (): string => useWorkspaceStoreContext((s) => s.activePaneId)

export const usePaneActions = (): PaneActions => useWorkspaceStoreContext((s) => s.paneActions)

export const usePaneById = (paneId: string): PaneGroup | null =>
  useWorkspaceStoreContext((s) => s.panes[paneId] ?? null)

/**
 * How many panes the user can actually see and switch between right now.
 *
 * Counts the leaves of BOTH layouts — the bottom panel is a pane you can focus, so when
 * it is open there really are two — but only while it is visible, and never while a pane
 * is fullscreened, since then exactly one is on screen.
 *
 * Returns a number rather than a layout object so the selector stays referentially
 * stable and does not re-render every consumer on unrelated store writes.
 */
export const useVisiblePaneCount = (): number => {
  const rootLeaves = useWorkspaceStoreContext((s) => getAllLeafIds(s.rootLayout).length)
  const bottomLeaves = useWorkspaceStoreContext((s) => getAllLeafIds(s.bottomLayout).length)
  const fullscreenPaneId = useWorkspaceStoreContext((s) => s.fullscreenPaneId)
  const isBottomPaneVisible = useUIState((s) => s.isBottomPaneVisible)

  if (fullscreenPaneId) return 1
  return rootLeaves + (isBottomPaneVisible ? bottomLeaves : 0)
}
