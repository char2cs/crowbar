import { useCallback, useEffect, useState } from 'react'
import { readLocal, writeLocal } from '@/lib/local-storage'

export const SIDEBAR_MIN_PX = 250
export const SIDEBAR_MAX_PX = 640
const SIDEBAR_DEFAULT_PX = 294

function loadSidebarWidth(): number {
  const stored = parseInt(readLocal('sidebar-width') ?? '', 10)
  // Clamped at BOTH ends: the peek card takes this number raw, so an over-large
  // stored value (stale, hand-edited) would render it past the opposite edge.
  return Number.isFinite(stored)
    ? Math.min(SIDEBAR_MAX_PX, Math.max(SIDEBAR_MIN_PX, stored))
    : SIDEBAR_DEFAULT_PX
}

function loadSidebarOpen(): boolean {
  return readLocal('sidebar-open') !== 'false'
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
    writeLocal('sidebar-open', String(sidebarOpen))
  }, [sidebarOpen])

  /** Persist one completed separator/keyboard gesture, never a resize frame. */
  const commitPreferredWidth = useCallback((requestedWidth: number) => {
    const width = Math.min(SIDEBAR_MAX_PX, Math.max(SIDEBAR_MIN_PX, Math.round(requestedWidth)))
    setPreferredWidth(width)
    writeLocal('sidebar-width', String(width))
  }, [])

  return {
    sidebarOpen,
    setSidebarOpen,
    preferredWidth,
    commitPreferredWidth,
  }
}
