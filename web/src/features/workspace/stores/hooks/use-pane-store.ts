import { useStore } from 'zustand'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WindowPaneState } from '@/features/panes/stores/window-pane-store.types'
import type { PaneActions } from '@/features/panes/stores/slices/pane-slice'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'

/**
 * Task 26: panes are window-level now (`windowPaneStore`, created once, never
 * destroyed on workspace switch) — every one of these used to read off the
 * ambient PER-WORKSPACE `useWorkspaceStoreContext()`. Callers of these hooks
 * needed no changes: the selector shapes are unchanged, only what they read
 * from is.
 */
function usePaneStore<T>(selector: (state: WindowPaneState) => T): T {
  return useStore(windowPaneStore, selector)
}

export const useRootLayout = (): LayoutNode => usePaneStore((s) => s.rootLayout)

export const useFullscreenPaneId = (): string | null => usePaneStore((s) => s.fullscreenPaneId)

export const useActivePaneId = (): string => usePaneStore((s) => s.activePaneId)

export const usePaneActions = (): PaneActions => usePaneStore((s) => s.paneActions)

export const usePaneById = (paneId: string): PaneGroup | null =>
  usePaneStore((s) => s.panes[paneId] ?? null)

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
  const rootLeaves = usePaneStore((s) => getAllLeafIds(s.rootLayout).length)
  const bottomLeaves = usePaneStore((s) => getAllLeafIds(s.bottomLayout).length)
  const fullscreenPaneId = usePaneStore((s) => s.fullscreenPaneId)
  const isBottomPaneVisible = useUIState((s) => s.isBottomPaneVisible)

  if (fullscreenPaneId) return 1
  return rootLeaves + (isBottomPaneVisible ? bottomLeaves : 0)
}
