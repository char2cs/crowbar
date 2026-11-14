import { useStore } from 'zustand'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WindowPaneState } from '@/features/panes/stores/window-pane-store.types'
import type { PaneActions } from '@/features/panes/stores/slices/pane-slice'
import type { PaneGroup, LayoutNode, ViewRecord } from '@/features/panes/types/pane'
import { showingLayout } from '@/features/panes/lib/view-state'

// Panes are window-level: one `windowPaneStore`, never destroyed on a workspace switch.
function usePaneStore<T>(selector: (state: WindowPaneState) => T): T {
  return useStore(windowPaneStore, selector)
}

/** Every view record by id. Stable across writes that touch no record. */
export const useViews = (): Record<string, ViewRecord> => usePaneStore((s) => s.views)

export const useStageLayout = (): LayoutNode => usePaneStore((s) => s.stage)

export const useActiveViewId = (): string | null => usePaneStore((s) => s.activeViewId)

export const useFullscreenPaneId = (): string | null => usePaneStore((s) => s.fullscreenPaneId)

export const useActivePaneId = (): string => usePaneStore((s) => s.activePaneId)

// A boolean, deliberately: subscribing to `useActivePaneId` would re-render every pane (and its
// chat) on each focus move instead of just the two that swapped.
export const useIsActivePane = (paneId: string): boolean =>
  usePaneStore((s) => s.activePaneId === paneId)

export const usePaneActions = (): PaneActions => usePaneStore((s) => s.paneActions)

export const usePaneById = (paneId: string): PaneGroup | null =>
  usePaneStore((s) => s.panes[paneId] ?? null)

// Panes the user can see and switch between: both layouts' leaves (the bottom panel counts only
// while visible), or 1 when fullscreened. A number, so selectors stay referentially stable.
export const useVisiblePaneCount = (): number => {
  const rootLeaves = usePaneStore((s) => getAllLeafIds(showingLayout(s)).length)
  const bottomLeaves = usePaneStore((s) => getAllLeafIds(s.bottomLayout).length)
  const fullscreenPaneId = usePaneStore((s) => s.fullscreenPaneId)
  const isBottomPaneVisible = useUIState((s) => s.isBottomPaneVisible)

  if (fullscreenPaneId) return 1
  return rootLeaves + (isBottomPaneVisible ? bottomLeaves : 0)
}
