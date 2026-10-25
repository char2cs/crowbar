import { useStore } from 'zustand'
import { selectProjectViewIds } from '../lib/view-selectors'
import { windowPaneStore } from '../stores/window-pane-store'

// Module-level: a fresh [] per selector call would never settle `useStore`.
const NO_VIEWS: readonly string[] = []

/** The active project's view ids in band order; stable until a view comes, goes or moves. */
export function useProjectViewIds(): readonly string[] {
  return useStore(windowPaneStore, (s) =>
    s.activeProjectId ? selectProjectViewIds(s, s.activeProjectId) : NO_VIEWS,
  )
}

/** Whether the view-tab strip is drawn above the panes, pushing them off the window's top. */
export function useHasViewStrip(): boolean {
  return useProjectViewIds().length > 0
}
