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
  // focused pane), --secondary only for the active-pane accent. 2px, not
  // 1px — reported live as too thin to read as a real boundary between
  // chats/panes.
  const BORDER = showActiveBorder ? '2px solid var(--secondary)' : '2px solid var(--border)'
  const NONE = 'none'
  const R = 'var(--radius-lg)'
  const ZERO = '0'
  // §7.4: inset on every edge, given up wherever the pane touches the outer
  // boundary of the layout area at all — a real window edge OR a side the
  // sidebar is shielding (radius/border stay put there; only the gutter
  // drops, so the sidebar and the pane sit flush with no gap between them).
  // Top has no shielding concept, so it always keeps its own inset.
  //
  // 1px, not 4px: two split neighbours are ALSO separated by their own
  // resize sash (`PaneSash`, 6px wide) sitting between them as its own flex
  // sibling — 4px margin + 6px sash + 4px margin came to 14px, reported
  // live as "huge" next to the crisp 8px inset the real window top gets.
  // 1px + 6px sash + 1px lands on that SAME 8px — one gutter value used
  // consistently, rather than the pane-splitting sash silently adding its
  // own width on top of it.
  const GUTTER = '1px'
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

/**
 * The IDE shell's own border/rounding, reusing the SAME per-corner values
 * `buildPaneContentStyle` already computed for the shared box it sits
 * inside — rather than a caller re-deriving its own square-or-rounded call
 * for the corners it shares with that box, which is how a rounded interior
 * pane ended up with a sharp, un-rounded IDE-shell corner sitting inside it
 * (the IDE shell only ever reasoned about the ONE edge facing the chat).
 *
 * `facingChatEdge` is the one edge that's genuinely internal to the shared
 * box (never a window edge, whatever `outer` says about it) — its own two
 * corners are always rounded and bordered, exactly as before. The other
 * three edges' ROUNDING, and the two corners that don't touch
 * `facingChatEdge`, copy `outer` verbatim: if the shared box's own corner
 * there is square (a real window edge) or rounded (interior), the IDE
 * shell's matching corner reads the same way, since visually the two are
 * the same corner.
 *
 * Border COLOR is never copied, though — the IDE shell stays neutral
 * --border on every edge, whatever `outer` says, and only whether a border
 * shows there at all (vs. `none`, a real window edge) comes from `outer`.
 * Reported live: copying `outer`'s color verbatim onto the shell's
 * non-facing edges lit its own side border up alongside the shared box's
 * real perimeter, reading as a second, illuminated line rather than one
 * clean boundary. Only the CHAT's own boundary (`outer`'s real perimeter)
 * is allowed to carry the active-pane --secondary accent — the IDE shell is
 * never the surface that marks "which pane has focus," so it stays one
 * color always.
 */
export function buildInnerViewStyle(
  outer: CSSProperties,
  facingChatEdge: 'left' | 'right' | 'top',
): CSSProperties {
  const BORDER = '2px solid var(--border)'
  const R = 'var(--radius-lg)'
  // Whatever `outer` drew there (any width/color, or 'none' for a real
  // window edge) — this edge shows a border at all, or it doesn't; if it
  // does, it's always the neutral color, never `outer`'s active accent.
  const neutralize = (edge: CSSProperties['borderTop']) => (edge === 'none' ? 'none' : BORDER)
  const style: CSSProperties = {
    borderTop: neutralize(outer.borderTop),
    borderLeft: neutralize(outer.borderLeft),
    borderRight: neutralize(outer.borderRight),
    borderBottom: neutralize(outer.borderBottom),
    borderTopLeftRadius: outer.borderTopLeftRadius,
    borderTopRightRadius: outer.borderTopRightRadius,
    borderBottomLeftRadius: outer.borderBottomLeftRadius,
    borderBottomRightRadius: outer.borderBottomRightRadius,
  }
  if (facingChatEdge === 'left') {
    style.borderLeft = BORDER
    style.borderTopLeftRadius = R
    style.borderBottomLeftRadius = R
  } else if (facingChatEdge === 'right') {
    style.borderRight = BORDER
    style.borderTopRightRadius = R
    style.borderBottomRightRadius = R
  } else {
    style.borderTop = BORDER
    style.borderTopLeftRadius = R
    style.borderTopRightRadius = R
  }
  return style
}
