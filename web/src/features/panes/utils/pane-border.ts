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

export function buildPaneContentStyle(
  position: PanePosition,
  sidebarSide: 'left' | 'right',
): CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide)
  const BORDER = '1px solid var(--border)'
  const NONE = 'none'
  const R = 'var(--radius-xl)'
  const ZERO = '0'

  return {
    borderTop: NONE,
    borderLeft: we('left') ? NONE : BORDER,
    borderRight: we('right') ? NONE : BORDER,
    borderBottom: we('bottom') ? NONE : BORDER,
    borderTopLeftRadius: we('left') ? ZERO : R,
    borderTopRightRadius: we('right') ? ZERO : R,
    borderBottomLeftRadius: we('left') || we('bottom') ? ZERO : R,
    borderBottomRightRadius: we('right') || we('bottom') ? ZERO : R,
  }
}
