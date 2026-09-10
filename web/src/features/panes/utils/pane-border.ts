import type { CSSProperties } from 'react'
import { IS_MAC } from '@/utils/platform'
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
  // Constant width so toggling never shifts layout. Neutral --border at rest
  // (Athas's glass-island keeps the same border/70 whether or not it's the
  // focused pane), --secondary only for the active-pane accent.
  const BORDER = showActiveBorder ? '1px solid var(--secondary)' : '1px solid var(--border)'
  const NONE = 'none'
  const R = 'var(--radius-lg)'
  const ZERO = '0'
  // §7.4: 4px inset on every edge, given up wherever the pane touches the
  // outer boundary of the layout area at all — a real window edge OR a side
  // the sidebar is shielding (radius/border stay put there; only the gutter
  // drops, so the sidebar and the pane sit flush with no gap between them).
  // Top has no shielding concept, so it always keeps its own inset. Two
  // neighbours across a split (touching neither boundary) each keep their
  // own facing edge's 4px, landing 8px apart.
  const GUTTER = '4px'
  // The pane actually touching the window's top — not a split neighbour
  // stacked below one — matches the gap the header row's OWN icons (traffic
  // lights, back/forward, sidebar toggle) sit at above the window's top
  // edge, not that row's full height. Those icon-sm buttons render 28px
  // (button-variants.ts `sm:size-7`) centered in the 44px/34px row
  // (SidebarProjectHeader), so half the leftover height is that inset.
  const HEADER_ROW_HEIGHT = IS_MAC ? 44 : 34
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
