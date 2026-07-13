import type { CSSProperties } from 'react'
import type { PanePosition } from '../types/pane'

type Edge = 'left' | 'top' | 'right' | 'bottom'

export function isWindowEdge(
  edge: Edge,
  position: PanePosition,
  sidebarSide: 'left' | 'right',
): boolean {
  switch (edge) {
    case 'top':
      return false
    case 'left':
      return position.atLeft && sidebarSide !== 'left'
    case 'right':
      return position.atRight && sidebarSide !== 'right'
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
): CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide)
  // Constant width (transparent when not drawn) so toggling never shifts layout;
  // 2px matches the tab-drag ring (ring-2 ring-secondary).
  const BORDER = showActiveBorder ? '2px solid var(--secondary)' : '2px solid transparent'
  const NONE = 'none'
  const R = 'var(--radius-lg)'
  const ZERO = '0'

  return {
    borderTop: BORDER,
    borderLeft: we('left') ? NONE : BORDER,
    borderRight: we('right') ? NONE : BORDER,
    borderBottom: we('bottom') ? NONE : BORDER,
    borderTopLeftRadius: we('left') ? ZERO : R,
    borderTopRightRadius: we('right') ? ZERO : R,
    borderBottomLeftRadius: we('left') || we('bottom') ? ZERO : R,
    borderBottomRightRadius: we('right') || we('bottom') ? ZERO : R,
  }
}
