import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'
import type { PanelSize } from 'react-resizable-panels'

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

/**
 * Open/collapsed state and remembered width for the sidebar panel, and the rules
 * for deciding which resizes are the user's intent.
 *
 * @param panelGroupRef the resizable group element, measured to tell a separator
 *   drag (group width fixed) from a window resize (group width changes).
 */
export function useSidebarPanel(panelGroupRef: RefObject<HTMLDivElement | null>) {
  const [sidebarOpen, setSidebarOpen] = useState(loadSidebarOpen)
  // The width the USER chose, as opposed to whatever width the panel happens to
  // have right now. It is what the sidebar comes back at when the panel
  // re-registers (a side flip), what the hover-peek card is sized to, and what
  // survives in storage across restarts.
  const [preferredWidth, setPreferredWidth] = useState(loadSidebarWidth)
  // Group width as of the last sidebar resize we saw, and whether the resizes
  // currently arriving are the window's doing. Latched rather than derived
  // per-event because ONE window resize emits TWO of them at the same group
  // width: first the sidebar's existing percentage applied to the new width,
  // then `preserve-pixel-size` correcting that back to pixels (measured, on a
  // 1714px → 700px narrowing: 245px at 35%, then 559px at 80%). Only the first
  // carries a changed group width, so a per-event test lets the second through
  // as if the user had chosen it.
  const lastGroupWidthRef = useRef<number | null>(null)
  const isWindowDrivenRef = useRef(true)
  /** Width a drag has reached, held out of state until the drag settles. */
  const pendingWidthRef = useRef<number | null>(null)

  // Remember whether the sidebar was left open, so a reload comes back the way
  // it was left instead of always docking it.
  useEffect(() => {
    try {
      localStorage.setItem('sidebar-open', String(sidebarOpen))
    } catch {
      // storage unavailable
    }
  }, [sidebarOpen])

  /**
   * Clears the window-driven latch: the resizes that follow a pointer going down
   * may be a separator drag, which is the one thing allowed to redefine the
   * remembered width.
   */
  const notePointerDown = useCallback(() => {
    isWindowDrivenRef.current = false
  }, [])

  // Only a separator drag expresses a WIDTH PREFERENCE. Everything else that
  // resizes the sidebar is a consequence of something else, and persisting those
  // indiscriminately — as this used to, on every resize tick — let one narrow
  // window redefine the sidebar's width for good: at a 700px window the content
  // pane's 20% minimum squeezes a 640px sidebar down to 559px, and 559px was
  // then written to storage as if the user had chosen it.
  const handleSidebarResize = useCallback(
    (size: PanelSize, _id: unknown, prevSize: PanelSize | undefined) => {
      const isCollapsed = size.asPercentage === 0
      setSidebarOpen((prev) => (!isCollapsed !== prev ? !isCollapsed : prev))
      if (isCollapsed || size.inPixels <= 0) return

      const groupWidth = panelGroupRef.current?.offsetWidth ?? 0
      if (lastGroupWidthRef.current !== null && lastGroupWidthRef.current !== groupWidth) {
        isWindowDrivenRef.current = true
      }
      lastGroupWidthRef.current = groupWidth

      // 1. The window is resizing us. A separator drag moves the divider inside a
      //    group whose own width never changes, so the latch cannot confuse the
      //    two — and note which way it fails: a missed pointer-down means a width
      //    is not remembered, never that a squeezed one is remembered as a choice.
      // 2. `prevSize` is absent on a panel's first resize: mount, and the
      //    re-registration a side flip causes.
      // 3. Growing back out of the collapsed state restores whatever width the
      //    panel had when it was collapsed, which may itself have been squeezed.
      if (isWindowDrivenRef.current || prevSize === undefined || prevSize.asPercentage === 0) return

      // Recorded to a ref, NOT to state, and not written to storage yet. This
      // runs on every frame of a drag; a setState here re-rendered the whole
      // shell and a synchronous localStorage write blocked it, ~90 times per
      // drag. It is committed once, when the layout settles.
      pendingWidthRef.current = Math.round(size.inPixels)
    },
    [panelGroupRef],
  )

  /** Flush the width a drag landed on, once, after the pointer is released. */
  const commitPreferredWidth = useCallback(() => {
    const width = pendingWidthRef.current
    if (width === null) return
    pendingWidthRef.current = null
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
    notePointerDown,
    handleSidebarResize,
    commitPreferredWidth,
  }
}
