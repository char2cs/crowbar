import { useStore } from 'zustand'
import { useSettingsStore } from '@/features/settings/store'
import { selectProjectViewIds } from '../lib/view-selectors'
import { windowPaneStore } from '../stores/window-pane-store'

const NO_VIEWS: readonly string[] = []

export function useTabViewIds(): readonly string[] {
  const scope = useSettingsStore((s) => s.settings.tabListScope)
  return useStore(windowPaneStore, (s) => {
    if (scope === 'all') return s.viewOrder
    return s.activeProjectId ? selectProjectViewIds(s, s.activeProjectId) : NO_VIEWS
  })
}

export function useHasViewStrip(): boolean {
  return useTabViewIds().length > 0
}
