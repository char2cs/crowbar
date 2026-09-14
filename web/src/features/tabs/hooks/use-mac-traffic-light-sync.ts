import { useEffect } from 'react'
import { setTrafficLightPosition } from '@/lib/crowbar-bridge'
import { IS_MAC } from '@/utils/platform'
import { EDGE_THRESHOLD } from './use-pane-top-row-edges'

// tauri.conf.json's config-time-only trafficLightPosition, tuned for
// SidebarProjectHeader: x mirrors its own pl-3 (12px); y centers in its
// 44px row. Sidebar-left must reproduce this EXACTLY, on every apply — not
// just "leave it alone" — so a value moved at runtime by a PRIOR
// sidebar-right session never drifts and sticks after switching back.
const STATIC_X = 12
const STATIC_Y = 23

/** The one pane-top-row actually sitting at the window's physical top-left
 *  corner right now, or null if none is (empty stage, still loading). Same
 *  `< EDGE_THRESHOLD` test `usePaneTopRowEdges` uses per-row, run globally
 *  since this hook has no single row's ref to read. */
function findTopLeftPaneTopRow(): DOMRect | null {
  const rows = document.querySelectorAll<HTMLElement>('[data-testid="pane-top-row"]')
  for (const row of rows) {
    const rect = row.getBoundingClientRect()
    if (rect.left < EDGE_THRESHOLD && rect.top < EDGE_THRESHOLD) return rect
  }
  return null
}

/**
 * Sidebar-right moves the window's true top-left corner off
 * `SidebarProjectHeader` (which the static config position was tuned for)
 * onto whichever pane's `PaneTopRow` now occupies it — a different
 * component, offset from the window's top edge by the pane box's own
 * margin+border (pane-border.ts), not flush at 0 like the sidebar header.
 * Both rows share the same 44px height/vertical-centering (IS_MAC), so
 * translating the static (x, y) by that row's own live top-left offset is
 * exact without re-deriving the AppKit inset math from scratch.
 *
 * Sidebar-left restores the exact static value instead of merely skipping
 * the call, so a window that had sidebar-right in a previous session (a
 * runtime move persists — there is no "reset to config" on the OS side)
 * still lands back on it.
 */
export function useMacTrafficLightSync(sidebarPosition: 'left' | 'right'): void {
  useEffect(() => {
    if (!IS_MAC) return

    function apply(): boolean {
      if (sidebarPosition !== 'right') {
        void setTrafficLightPosition(STATIC_X, STATIC_Y)
        return true
      }
      const rect = findTopLeftPaneTopRow()
      if (!rect) return false
      void setTrafficLightPosition(rect.left + STATIC_X, rect.top + STATIC_Y)
      return true
    }

    let observer: MutationObserver | null = null
    if (!apply()) {
      // Cold boot: settings can rehydrate to 'right' and re-fire this effect
      // before the pane tree (its own async mount) has put a pane-top-row at
      // the top-left corner. Without a retry the window is stuck at the
      // config-time (left) position forever — watch the DOM until one shows
      // up instead of only reacting to resize.
      observer = new MutationObserver(() => {
        if (apply()) observer?.disconnect()
      })
      observer.observe(document.body, { childList: true, subtree: true })
    }

    window.addEventListener('resize', apply)
    return () => {
      window.removeEventListener('resize', apply)
      observer?.disconnect()
    }
  }, [sidebarPosition])
}
