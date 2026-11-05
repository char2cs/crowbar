import type { CSSProperties } from 'react'
import type { PanePosition } from '../types/pane'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

type Edge = 'left' | 'top' | 'right' | 'bottom'

export function isWindowEdge(
  edge: Edge,
  position: PanePosition,
  sidebarSide: 'left' | 'right',
  sidebarOpen: boolean = true,
): boolean {
  // The sidebar shields a pane only while it is on screen; collapsed, the pane IS the window
  // edge. A rounded, bordered corner against the window's vibrant frame costs WKWebView
  // ~100ms per composited frame.
  const shielded = (side: 'left' | 'right') => sidebarOpen && sidebarSide === side
  switch (edge) {
    case 'top':
      return false
    case 'left':
      return position.atLeft && !shielded('left')
    case 'right':
      return position.atRight && !shielded('right')
    case 'bottom':
      return position.atBottom
  }
}

/** @param showActiveBorder Draw the accent ring — not "this pane is active": a lone pane has
 *   nothing to stand out from, so the caller decides (useVisiblePaneCount); this only draws. */
export function buildPaneContentStyle(
  position: PanePosition,
  sidebarSide: 'left' | 'right',
  showActiveBorder: boolean,
  sidebarOpen: boolean = true,
): CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide, sidebarOpen)
  // Constant width so toggling never shifts layout; neutral --border at rest,
  // --secondary only for the active-pane accent.
  const BORDER = showActiveBorder ? '1px solid var(--secondary)' : '1px solid var(--border)'
  const NONE = 'none'
  const R = 'var(--radius-lg)'
  const ZERO = '0'
  // The gutter drops on any side touching the layout's outer boundary (window edge or the
  // sidebar), so they sit flush (§7.4). 1px because the 6px PaneSash sits between split
  // neighbours: 1 + 6 + 1 matches the 8px inset at the window top.
  const GUTTER = '1px'
  // A pane at the window top is inset like the header row's 28px icon buttons, which sit
  // centred in the chrome row — half its leftover height.
  const HEADER_ROW_HEIGHT = CHROME_ROW_HEIGHT_PX
  const HEADER_ICON_HEIGHT = 28
  const TOP_GUTTER = position.atTop ? `${(HEADER_ROW_HEIGHT - HEADER_ICON_HEIGHT) / 2}px` : GUTTER

  return {
    borderTop: BORDER,
    borderLeft: we('left') ? NONE : BORDER,
    borderRight: we('right') ? NONE : BORDER,
    borderBottom: we('bottom') ? NONE : BORDER,
    borderTopLeftRadius: we('left') ? ZERO : R,
    borderTopRightRadius: we('right') ? ZERO : R,
    borderBottomLeftRadius: we('left') || we('bottom') ? ZERO : R,
    borderBottomRightRadius: we('right') || we('bottom') ? ZERO : R,
    marginLeft: position.atLeft ? ZERO : GUTTER,
    marginTop: we('top') ? ZERO : TOP_GUTTER,
    marginRight: position.atRight ? ZERO : GUTTER,
    marginBottom: we('bottom') ? ZERO : GUTTER,
  }
}

// The IDE shell borders only the seam facing its pane's chat: its other edges coincide with the
// pane box's own border, and drawing them would double that line. Never rounded, so it nests
// inside the box's corners whatever their radius.
export function buildInnerViewStyle(facingChatEdge: 'left' | 'right' | 'top'): CSSProperties {
  const BORDER = '1px solid var(--border)'
  const ZERO = '0'
  const NONE = 'none'

  return {
    borderTop: facingChatEdge === 'top' ? BORDER : NONE,
    borderLeft: facingChatEdge === 'left' ? BORDER : NONE,
    borderRight: facingChatEdge === 'right' ? BORDER : NONE,
    borderBottom: NONE,
    borderTopLeftRadius: ZERO,
    borderTopRightRadius: ZERO,
    borderBottomLeftRadius: ZERO,
    borderBottomRightRadius: ZERO,
  }
}
