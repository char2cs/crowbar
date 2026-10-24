import { useEffect } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { SIDEBAR_PANEL_TABS } from './sidebar-panel-tabs'

/** The mode a pane surface pulls toward, from its `data-sidebar-surface` marker. */
function surfaceOf(target: EventTarget | null) {
  if (!(target instanceof Element)) return null
  const kind = target.closest('[data-sidebar-surface]')?.getAttribute('data-sidebar-surface')
  return SIDEBAR_PANEL_TABS.find((t) => t.tab === kind) ?? null
}

/**
 * Focus crossing into a different kind of pane surface (editor, diff, chat or
 * terminal) moves the sidebar mode. Unmarked focus (a dialog, the mode bar)
 * is no crossing, so a manual choice holds until the next one. Focus before
 * the user's first press (startup restoring an editor, a terminal
 * auto-focusing) is not theirs, so the persisted mode survives it.
 */
export function useSidebarFocusMode(): void {
  useEffect(() => {
    // The kind focus last entered, not the mode: comparing against the mode
    // would undo a manual choice on the next focus within the same surface.
    let lastKind = surfaceOf(document.activeElement)?.tab ?? null
    // No isTrusted check: startup focus is programmatic, never a press, and
    // jsdom cannot produce trusted events for the tests that drive this gate.
    let interacted = false
    const onInteract = () => {
      interacted = true
    }
    const onFocusIn = (event: FocusEvent) => {
      const surface = surfaceOf(event.target)
      if (!surface || surface.tab === lastKind) return
      lastKind = surface.tab
      if (!interacted || !surface.available(useFocusedWorkspaceContextStore.getState())) return
      useSidebarStore.getState().setActiveTab(surface.tab)
    }
    document.addEventListener('pointerdown', onInteract, true)
    document.addEventListener('keydown', onInteract, true)
    document.addEventListener('focusin', onFocusIn)
    return () => {
      document.removeEventListener('pointerdown', onInteract, true)
      document.removeEventListener('keydown', onInteract, true)
      document.removeEventListener('focusin', onFocusIn)
    }
  }, [])
}
