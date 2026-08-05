import { useCallback, useEffect, useState } from 'react'

export const SIDEBAR_MIN_PX = 250
export const SIDEBAR_MAX_PX = 640
const SIDEBAR_DEFAULT_PX = 294

function loadSidebarWidth(): number {
  try {
    const stored = parseInt(localStorage.getItem('sidebar-width') ?? '', 10)
    // Clamped at BOTH ends. The panel is bounded by its own minSize/maxSize, but
    // the peek card takes this number raw — an over-large stored value (stale,
    // hand-edited) would render it past the opposite window edge.
    return Number.isFinite(stored)
      ? Math.min(SIDEBAR_MAX_PX, Math.max(SIDEBAR_MIN_PX, stored))
      : SIDEBAR_DEFAULT_PX
  } catch {
    return SIDEBAR_DEFAULT_PX
  }
}

function loadSidebarOpen(): boolean {
  try {
    return localStorage.getItem('sidebar-open') !== 'false'
  } catch {
    return true
  }
}

/** Open/collapsed state and the last width explicitly chosen by the user. */
export function useSidebarPanel() {
  const [sidebarOpen, setSidebarOpen] = useState(loadSidebarOpen)
  // Window pressure never writes this value: CSS temporarily squeezes the live
  // track, then restores the user's preference when space returns.
  const [preferredWidth, setPreferredWidth] = useState(loadSidebarWidth)

  // Remember whether the sidebar was left open, so a reload comes back the way
  // it was left instead of always docking it.
  useEffect(() => {
    try {
      localStorage.setItem('sidebar-open', String(sidebarOpen))
    } catch {
      // storage unavailable
    }
  }, [sidebarOpen])

  /** Persist one completed separator/keyboard gesture, never a resize frame. */
  const commitPreferredWidth = useCallback((requestedWidth: number) => {
    const width = Math.min(SIDEBAR_MAX_PX, Math.max(SIDEBAR_MIN_PX, Math.round(requestedWidth)))
    setPreferredWidth(width)
    try {
      localStorage.setItem('sidebar-width', String(width))
    } catch {
      // storage unavailable
    }
  }, [])

  return {
    sidebarOpen,
    setSidebarOpen,
    preferredWidth,
    commitPreferredWidth,
  }
}
