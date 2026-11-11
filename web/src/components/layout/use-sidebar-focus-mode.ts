import { useEffect, useRef } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { SIDEBAR_PANEL_TABS } from './sidebar-panel-tabs'

/** The mode a pane surface pulls toward, from its `data-sidebar-surface` marker. */
function surfaceOf(target: EventTarget | null) {
  if (!(target instanceof Element)) return null
  const kind = target.closest('[data-sidebar-surface]')?.getAttribute('data-sidebar-surface')
  return SIDEBAR_PANEL_TABS.find((t) => t.tab === kind) ?? null
}

// With `sidebarFollowsFocus`, focus crossing into another pane surface kind moves the sidebar
// mode. Unmarked focus is no crossing, so a manual choice holds; focus before the user's first
// press (startup restore) is not theirs, so the persisted mode survives it.
export function useSidebarFocusMode(): void {
  const follows = useSettingsStore((s) => s.settings.sidebarFollowsFocus)
  // No isTrusted check (startup focus is never a press; jsdom can't make trusted events). Tracked
  // apart from the focus listener so flipping the setting doesn't reset to "startup".
  const interacted = useRef(false)
  useEffect(() => {
    const onInteract = () => {
      interacted.current = true
    }
    document.addEventListener('pointerdown', onInteract, true)
    document.addEventListener('keydown', onInteract, true)
    return () => {
      document.removeEventListener('pointerdown', onInteract, true)
      document.removeEventListener('keydown', onInteract, true)
    }
  }, [])
  useEffect(() => {
    if (!follows) return
    // The kind focus last entered, not the mode: comparing against the mode
    // would undo a manual choice on the next focus within the same surface.
    let lastKind = surfaceOf(document.activeElement)?.tab ?? null
    const onFocusIn = (event: FocusEvent) => {
      const surface = surfaceOf(event.target)
      if (!surface || surface.tab === lastKind) return
      lastKind = surface.tab
      if (!interacted.current || !surface.available(useFocusedWorkspaceContextStore.getState())) {
        return
      }
      treeUiStore.getState().setActiveTab(surface.tab)
    }
    document.addEventListener('focusin', onFocusIn)
    return () => document.removeEventListener('focusin', onFocusIn)
  }, [follows])
}
