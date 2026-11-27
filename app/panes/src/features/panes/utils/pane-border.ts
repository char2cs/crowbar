import type { CSSProperties } from 'react'
import type { PanePosition } from '../types/pane'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

type Edge = 'left' | 'top' | 'right' | 'bottom'

function isWindowEdge(
  edge: Edge,
  position: PanePosition,
  sidebarSide: 'left' | 'right',
  sidebarOpen: boolean = true,
): boolean {
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

export function buildPaneContentStyle(
  position: PanePosition,
  sidebarSide: 'left' | 'right',
  showActiveBorder: boolean,
  sidebarOpen: boolean = true,
): CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide, sidebarOpen)
  const BORDER = showActiveBorder
    ? 'var(--pane-accent-width) solid var(--pane-accent)'
    : '1px solid var(--pane-border)'
  const NONE = 'none'
  const R = 'var(--radius-lg)'
  const ZERO = '0'
  const GUTTER = '1px'
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
