import { useStore } from 'zustand'
import { useSettingsStore } from '@/features/settings/store'
import { selectProjectViewIds } from '../lib/view-selectors'
import { windowPaneStore } from '../stores/window-pane-store'

// Module-level: a fresh [] per selector call would never settle `useStore`.
const NO_VIEWS: readonly string[] = []

/**
 * The view ids the tab strip lists, in band order: the active project's, or with the
 * `tabListScope` setting on 'all', every view open on this client. Stable until one comes, goes or moves.
 */
export function useTabViewIds(): readonly string[] {
  const scope = useSettingsStore((s) => s.settings.tabListScope)
  return useStore(windowPaneStore, (s) => {
    if (scope === 'all') return s.viewOrder
    return s.activeProjectId ? selectProjectViewIds(s, s.activeProjectId) : NO_VIEWS
  })
}

/** Whether the view-tab strip is drawn above the panes, pushing them off the window's top. */
export function useHasViewStrip(): boolean {
  return useTabViewIds().length > 0
}
