import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WindowPaneState } from '@/features/panes/stores/window-pane-store.types'
import type { PaneGroup } from '@/features/panes/types/pane'

// Panes are window-level: one `windowPaneStore`, never destroyed on a workspace switch.
function usePaneStore<T>(selector: (state: WindowPaneState) => T): T {
  return useStore(windowPaneStore, selector)
}

export const useFullscreenPaneId = (): string | null => usePaneStore((s) => s.fullscreenPaneId)

export const useActivePaneId = (): string => usePaneStore((s) => s.activePaneId)

export const usePaneById = (paneId: string): PaneGroup | null =>
  usePaneStore((s) => s.panes[paneId] ?? null)
