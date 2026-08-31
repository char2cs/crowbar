import type { CSSProperties } from 'react'
import type { PanePosition } from '../types/pane'

type Edge = 'left' | 'top' | 'right' | 'bottom'

export function isWindowEdge(
  edge: Edge,
  position: PanePosition,
  sidebarSide: 'left' | 'right',
  sidebarOpen: boolean = true,
): boolean {
  // The sidebar only shields a pane from the window frame while it is actually
  // on screen. Collapsed, the pane IS the window edge — and asking only which
  // SIDE the sidebar is on, never whether it is there, left a rounded corner and
  // a border hard against the frame whenever the sidebar was hidden. That corner
  // lands on the window's own rounded, vibrant edge, and compositing it cost
  // ~98ms per frame in WKWebView: 8ms frames became 106ms (125fps → 9fps) for
  // as long as the sidebar stayed hidden.
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

/**
 * @param showActiveBorder Draw the accent ring. NOT the same as "this pane is active":
 *   with a single pane on screen there is nothing to distinguish it FROM, so the ring is
 *   pure noise. The caller decides (see useVisiblePaneCount) — this only draws.
 */
export function buildPaneContentStyle(
  position: PanePosition,
  sidebarSide: 'left' | 'right',
  showActiveBorder: boolean,
  sidebarOpen: boolean = true,
): CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide, sidebarOpen)
  // Constant width (transparent when not drawn) so toggling never shifts layout;
  // 2px matches the tab-drag ring (ring-2 ring-secondary).
  const BORDER = showActiveBorder ? '2px solid var(--secondary)' : '2px solid transparent'
  const NONE = 'none'
  const R = 'var(--radius-lg)'
  const ZERO = '0'
  // §7.4: 4px inset on every edge, given up wherever the pane actually meets
  // the window frame — the same we(edge) test the border/radius above use, so
  // top (never a window edge) and the sidebar-shielded side always keep it,
  // while a real window edge runs flush. Two neighbours across a split each
  // keep their own facing edge's 4px, landing 8px apart.
  const GUTTER = '4px'

  return {
    borderTop: BORDER,
    borderLeft: we('left') ? NONE : BORDER,
    borderRight: we('right') ? NONE : BORDER,
    borderBottom: we('bottom') ? NONE : BORDER,
    borderTopLeftRadius: we('left') ? ZERO : R,
    borderTopRightRadius: we('right') ? ZERO : R,
    borderBottomLeftRadius: we('left') || we('bottom') ? ZERO : R,
    borderBottomRightRadius: we('right') || we('bottom') ? ZERO : R,
    marginLeft: we('left') ? ZERO : GUTTER,
    marginTop: we('top') ? ZERO : GUTTER,
    marginRight: we('right') ? ZERO : GUTTER,
    marginBottom: we('bottom') ? ZERO : GUTTER,
  }
}
