import { useEffect } from 'react'
import { setTrafficLightPosition } from '@/lib/crowbar-bridge'
import { TRAFFIC_LIGHT_Y_PX } from '@/lib/chrome-metrics'
import { IS_MAC } from '@/utils/platform'
import { useMediaQuery } from '@/hooks/use-media-query'
import { EDGE_THRESHOLD } from './use-pane-top-row-edges'

// tauri.conf.json's static trafficLightPosition, tuned for SidebarProjectHeader (x = its 12px
// padding, y centred in its 44px row). Sidebar-left re-applies it on every run because a runtime
// move persists on the OS side, so a prior sidebar-right position would otherwise stick.
const STATIC_X = 12
const STATIC_Y = TRAFFIC_LIGHT_Y_PX

/** The visible pane-top-row at the window's top-left corner, or null. Zero-size rows are skipped:
 *  WorkspaceHost keeps visited workspaces mounted under `display:none`, and their all-zero rects
 *  would otherwise pass the `< EDGE_THRESHOLD` corner test ahead of the real row. */
function findTopLeftPaneTopRow(): DOMRect | null {
  const rows = document.querySelectorAll<HTMLElement>('[data-testid="pane-top-row"]')
  for (const row of rows) {
    const rect = row.getBoundingClientRect()
    if (rect.width === 0 || rect.height === 0) continue
    if (rect.left < EDGE_THRESHOLD && rect.top < EDGE_THRESHOLD) return rect
  }
  return null
}

/** Runs `fn` two animation frames from now; returns a canceller. */
function afterTwoFrames(fn: () => void): () => void {
  let first: number | null = null
  let second: number | null = null
  first = requestAnimationFrame(() => {
    first = null
    second = requestAnimationFrame(() => {
      second = null
      fn()
    })
  })
  return () => {
    if (first !== null) cancelAnimationFrame(first)
    if (second !== null) cancelAnimationFrame(second)
  }
}

/** Sidebar-right puts a pane's PaneTopRow at the window corner instead of SidebarProjectHeader;
 *  both are 44px rows, so the lights go to the static (x, y) shifted by that row's live offset.
 *  Re-runs on `themeKey` and OS colour scheme: applying a theme makes AppKit reset the buttons. */
export function useMacTrafficLightSync(sidebarPosition: 'left' | 'right', themeKey: string): void {
  const systemPrefersDark = useMediaQuery('(prefers-color-scheme: dark)')

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

    let cancelInitialApply: (() => void) | null = null
    // Cold boot: the pane tree mounts asynchronously, so no row may be at the corner yet; observe
    // the DOM until one appears instead of staying at the static position.
    let coldBootObserver: MutationObserver | null = null

    if (sidebarPosition === 'right') {
      // Two frames first: a theme switch's border/spacing reflow (async theme import) may still be
      // in flight, and an immediate read would measure the pre-switch box.
      cancelInitialApply = afterTwoFrames(() => {
        if (apply()) return
        coldBootObserver = new MutationObserver(() => {
          if (apply()) coldBootObserver?.disconnect()
        })
        coldBootObserver.observe(document.body, { childList: true, subtree: true })
      })
    } else if (!apply()) {
      coldBootObserver = new MutationObserver(() => {
        if (apply()) coldBootObserver?.disconnect()
      })
      coldBootObserver.observe(document.body, { childList: true, subtree: true })
    }

    window.addEventListener('resize', apply)
    return () => {
      window.removeEventListener('resize', apply)
      cancelInitialApply?.()
      coldBootObserver?.disconnect()
    }
  }, [sidebarPosition, themeKey, systemPrefersDark])
}
