import { useEffect } from 'react'
import { setTrafficLightPosition } from '@/lib/crowbar-bridge'
import { IS_MAC } from '@/utils/platform'
import { useMediaQuery } from '@/hooks/use-media-query'
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
 *  since this hook has no single row's ref to read.
 *
 *  WorkspaceHost keeps recently-visited workspaces mounted but hidden
 *  (`display:none`) so switching back is instant (its own doc) — a hidden
 *  row's `getBoundingClientRect()` degenerates to all-zero, which trivially
 *  passes `< EDGE_THRESHOLD` and reads as "flush against the top-left
 *  corner." Skipping zero-size rows keeps a hidden pane from ever winning
 *  that check ahead of the one real visible row, whatever order they mount
 *  in — live-reported as the traffic lights sticking at a fixed spot
 *  regardless of sidebar side, whichever hidden row happened to sort first
 *  in `querySelectorAll`. */
function findTopLeftPaneTopRow(): DOMRect | null {
  const rows = document.querySelectorAll<HTMLElement>('[data-testid="pane-top-row"]')
  for (const row of rows) {
    const rect = row.getBoundingClientRect()
    if (rect.width === 0 || rect.height === 0) continue
    if (rect.left < EDGE_THRESHOLD && rect.top < EDGE_THRESHOLD) return rect
  }
  return null
}

/** Runs `fn` two animation frames from now; returns a canceller. Sidebar-right's
 *  own comment below explains why its first measurement needs this. */
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
 *
 * `themeKey` must change on every real theme switch (e.g. `${theme}:${themeMode}`).
 * Applying a theme pins the vibrancy view's NSAppearance (set_vibrancy_appearance),
 * and AppKit relayouts the title bar's standard-button frames as a side effect of
 * that — undoing whatever this hook last set. Re-running on theme change reapplies
 * over that native reset; it does not touch the MutationObserver above, which
 * exists only for the separate cold-boot case (pane tree not mounted yet) and
 * still disconnects once it finds a row, on every effect run alike.
 *
 * `themeKey` alone misses one case: it comes from `useSettingsStore`, so it stays
 * literally "system" across an OS-level appearance flip (System Settings, or
 * macOS auto dark/light) — the app's own setting never changed, even though
 * settings-effects.ts resolves "system" against this exact media query and still
 * re-applies the theme (and resets the buttons) when it fires. Watching it here
 * too re-runs this effect for that case as well.
 */
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
    // Cold boot: settings can rehydrate to 'right' and re-fire this effect
    // before the pane tree (its own async mount) has put a pane-top-row at
    // the top-left corner. Without a retry the window is stuck at the
    // config-time (left) position forever — watch the DOM until one shows
    // up instead of only reacting to resize. Declared (not assigned) here so
    // both call sites below assign it directly, inline, rather than through
    // a shared helper's return value — the disconnect this effect's cleanup
    // calls is the exact same binding either branch set.
    let coldBootObserver: MutationObserver | null = null

    if (sidebarPosition === 'right') {
      // Live-measured, unlike the static left-side constants above — a theme
      // switch's new border width/spacing reflow (applyTheme's dynamic
      // theme-registry import in settings-effects.ts resolves asynchronously)
      // can still be in flight the instant this effect fires, which would read
      // the row's PRE-switch box. Give it two frames before the first read.
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
